// Package daemon owns the resident process: IPC, session state, the vault,
// policy evaluation, audit writes and pending confirmations.
package daemon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tyr-zhang/agent-password/internal/audit"
	"github.com/tyr-zhang/agent-password/internal/build"
	"github.com/tyr-zhang/agent-password/internal/crypto"
	"github.com/tyr-zhang/agent-password/internal/executor"
	"github.com/tyr-zhang/agent-password/internal/platform"
	"github.com/tyr-zhang/agent-password/internal/protocol"
	"github.com/tyr-zhang/agent-password/internal/session"
	"github.com/tyr-zhang/agent-password/internal/store"
)

var Version = build.Version

var errVaultMissing = errors.New("vault does not exist")

type Config struct {
	IdleTimeout time.Duration
	HardTimeout time.Duration
	ConfirmTTL  time.Duration
	FillTTL     time.Duration
}

func DefaultConfig() Config {
	return Config{
		IdleTimeout: session.DefaultIdleTimeout,
		HardTimeout: session.DefaultHardTimeout,
		ConfirmTTL:  session.DefaultConfirmTTL,
		FillTTL:     session.DefaultFillTTL,
	}
}

// ConfigFromEnv applies optional duration overrides to the defaults. Values
// are Go durations such as "12h" or "30m"; invalid values are ignored so a
// typo cannot accidentally lock the vault immediately.
func ConfigFromEnv() Config {
	cfg := DefaultConfig()
	apply := func(name string, target *time.Duration) {
		v := os.Getenv(name)
		if v == "" {
			return
		}
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			*target = d
		}
	}
	apply("AGENT_PWD_IDLE_TIMEOUT", &cfg.IdleTimeout)
	apply("AGENT_PWD_HARD_TIMEOUT", &cfg.HardTimeout)
	apply("AGENT_PWD_CONFIRM_TTL", &cfg.ConfirmTTL)
	apply("AGENT_PWD_FILL_TTL", &cfg.FillTTL)
	if cfg.HardTimeout < cfg.IdleTimeout {
		cfg.HardTimeout = cfg.IdleTimeout
	}
	return cfg
}

type Daemon struct {
	cfg         Config
	store       *store.Store
	sess        *session.Session
	redactor    *executor.Redactor
	exec        *executor.Executor
	auditPath   string
	pending     *pendingRegistry

	mu          sync.RWMutex
	auditBroken error
	ln          net.Listener
	ctx         context.Context
	cancel      context.CancelFunc
	stopOnce    sync.Once
	dataDir     string
}

func New(cfg Config) (*Daemon, error) {
	dir, err := platform.DataDir()
	if err != nil {
		return nil, err
	}
	d := &Daemon{
		cfg:       cfg,
		store:     store.New(filepath.Join(dir, "vault.dat")),
		redactor:  executor.NewRedactor(),
		auditPath: filepath.Join(dir, "audit.log"),
		dataDir:   dir,
	}
	d.exec = executor.New(d.redactor)
	d.sess = session.New(session.Config{
		IdleTimeout: cfg.IdleTimeout, HardTimeout: cfg.HardTimeout,
		ConfirmTTL: cfg.ConfirmTTL, FillTTL: cfg.FillTTL,
	})
	d.pending = newPendingRegistry(cfg.ConfirmTTL)
	d.sess.OnLock(func() {
		d.store.Lock()
		d.redactor.Reset()
		d.pending.cancelAll()
	})
	d.ctx, d.cancel = context.WithCancel(context.Background())
	return d, nil
}

// Run blocks serving IPC requests. The daemon is never started implicitly by
// a CLI action command.
func (d *Daemon) Run(ctx context.Context) error {
	if err := platform.Current().HardenProcess(); err != nil {
		return err
	}
	ln, err := platform.Current().Listen()
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.ln = ln
	d.mu.Unlock()
	if !store.Exists(d.store.Path()) {
		d.mu.Lock()
		d.auditBroken = errVaultMissing
		d.mu.Unlock()
	} else if report, err := audit.Verify(d.auditPath); err != nil || !report.OK {
		d.mu.Lock()
		if err != nil {
			d.auditBroken = err
		} else {
			d.auditBroken = errors.New(report.Problem)
		}
		d.mu.Unlock()
	}
	go func() {
		<-ctx.Done()
		d.Stop()
	}()
	go d.pending.cleanup(d.ctx)
	err = d.serve(ln)
	d.Stop()
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

func (d *Daemon) Stop() {
	d.stopOnce.Do(func() {
		d.cancel()
		d.mu.RLock()
		ln := d.ln
		d.mu.RUnlock()
		if ln != nil {
			_ = ln.Close()
		}
		d.sess.Lock()
		d.pending.cancelAll()
	})
}

func (d *Daemon) serve(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go d.handleConn(conn)
	}
}

func (d *Daemon) handleConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(6 * time.Minute))
	if err := platform.Current().CheckPeer(conn); err != nil {
		d.write(conn, protocol.Fail(protocol.CodeDenied, err.Error()))
		return
	}
	payload, err := protocol.ReadFrame(conn)
	if err != nil {
		return
	}
	var req protocol.Request
	if err := json.Unmarshal(payload, &req); err != nil {
		d.write(conn, protocol.Fail(protocol.CodeBadArgs, "invalid request"))
		return
	}
	if req.Args == nil {
		req.Args = map[string]any{}
	}
	req.Client.Name = sanitize(req.Client.Name, 160)
	req.Client.CWD = sanitize(req.Client.CWD, 260)
	resp := d.dispatch(req)
	d.write(conn, resp)
}

func (d *Daemon) write(conn net.Conn, resp protocol.Response) {
	payload, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_ = protocol.WriteFrame(conn, payload)
}

func sanitize(s string, limit int) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, s)
	if len(s) > limit {
		s = s[:limit]
	}
	return s
}

func (d *Daemon) dispatch(req protocol.Request) protocol.Response {
	switch req.Op {
	case "status", "unlock", "doctor", "audit.verify", "cred.restore":
		return d.handlePublic(req)
	}
	if _, _, err := d.sess.Keys(); err != nil {
		return protocol.Fail(protocol.CodeLocked, "vault is locked; run agent-pwd unlock")
	}
	switch req.Op {
	case "lock", "auth.rotate-password", "cred.add", "cred.list", "cred.show", "cred.rm",
		"policy.add", "policy.list", "policy.rm", "audit.tail":
		return d.handleAdmin(req)
	case "action.ssh", "action.sql", "action.api", "action.env", "fill.issue", "fill.apply", "fill.revoke":
		return d.handleAction(req)
	case "confirm.approve", "confirm.deny", "confirm.result", "confirm.info":
		return d.handleConfirm(req)
	default:
		return protocol.Fail(protocol.CodeBadArgs, "unknown operation: "+req.Op)
	}
}

func (d *Daemon) handlePublic(req protocol.Request) protocol.Response {
	switch req.Op {
	case "status":
		unlocked, expires, failures := d.sess.Status()
		result := map[string]any{
			"unlocked": unlocked, "version": Version, "data_dir": d.dataDir,
			"vault": d.store.Path(), "failures": failures, "session_id": d.sess.StableID(),
			"idle_timeout_seconds": int(d.cfg.IdleTimeout.Seconds()),
			"hard_timeout_seconds": int(d.cfg.HardTimeout.Seconds()),
			"confirm_timeout_seconds": int(d.cfg.ConfirmTTL.Seconds()),
			"fill_ttl_seconds": int(d.cfg.FillTTL.Seconds()),
			"pending": d.pending.count(), "service": platformServiceStatus(),
		}
		if !expires.IsZero() {
			result["expires_at"] = expires.UTC().Format(time.RFC3339)
		}
		if store.Exists(d.store.Path()) {
			if data, err := os.ReadFile(d.store.Path()); err == nil {
				if h, err := crypto.ParseHeader(data); err == nil {
					result["kdf"] = map[string]any{
						"time": h.KDF.Time, "memory_kib": h.KDF.MemoryKiB, "parallelism": h.KDF.Parallelism,
						"salt": base64.StdEncoding.EncodeToString(h.Salt[:]),
					}
				}
			}
		}
		d.mu.RLock()
		broken := d.auditBroken
		d.mu.RUnlock()
		result["audit_ok"] = broken == nil
		if broken != nil {
			result["audit_problem"] = broken.Error()
		}
		return protocol.OK(result)
	case "unlock":
		d.mu.RLock()
		broken := d.auditBroken
		d.mu.RUnlock()
		if errors.Is(broken, errVaultMissing) && store.Exists(d.store.Path()) {
			report, verr := audit.Verify(d.auditPath)
			if verr == nil && report.OK {
				d.mu.Lock()
				d.auditBroken = nil
				d.mu.Unlock()
				broken = nil
			}
		}
		if broken != nil {
			return protocol.Fail(protocol.CodeCorrupt, "audit chain invalid; refusing to unlock: "+broken.Error())
		}
		if remaining := d.sess.BackoffRemaining(); remaining > 0 {
			secs := int(remaining.Seconds()) + 1
			return protocol.FailRetry(protocol.CodeLocked,
				fmt.Sprintf("too many failed attempts; retry in %ds", secs), secs)
		}
		password := argString(req.Args, "password")
		body, err := d.store.Unlock(password)
		if err != nil {
			retry, stop := d.sess.Failure()
			msg := "unlock failed"
			if errors.Is(err, crypto.ErrBadPassword) {
				msg = "wrong master password"
			} else {
				msg = err.Error()
			}
			if stop {
				go d.Stop()
				return protocol.FailRetry(protocol.CodeInternal, msg+"; too many failed attempts, daemon stopping", int(retry.Seconds()))
			}
			return protocol.FailRetry(protocol.CodeLocked, msg, int(retry.Seconds()))
		}
		dek, kek := d.store.DEKKEK()
		d.sess.Unlock(dek, kek)
		crypto.Zero(dek)
		crypto.Zero(kek)
		d.redactor.Reset(secretsOf(body)...)
		return protocol.OK(map[string]any{"status": "unlocked", "credentials": len(body.Credentials)})
	case "audit.verify":
		report, err := audit.Verify(d.auditPath)
		if err != nil {
			return protocol.Fail(protocol.CodeInternal, err.Error())
		}
		return protocol.OK(map[string]any{"ok": report.OK, "records": report.Records,
			"problem": report.Problem, "first_seq": report.FirstSeq, "last_seq": report.LastSeq})
	case "cred.restore":
		p := argString(req.Args, "from")
		if p == "" {
			return protocol.Fail(protocol.CodeBadArgs, "missing --from")
		}
		if err := d.store.RestoreFrom(p); err != nil {
			return protocol.Fail(protocol.CodeInternal, err.Error())
		}
		return protocol.OK(map[string]any{"status": "restored", "from": p})
	case "doctor":
		return d.doctor(req)
	default:
		return protocol.Fail(protocol.CodeBadArgs, "unknown operation")
	}
}

func platformServiceStatus() string {
	s, err := platform.Current().ServiceStatus()
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(s)
}
