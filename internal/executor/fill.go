package executor

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/tyr-zhang/agent-password/internal/store"
	"github.com/tyr-zhang/agent-password/internal/session"
)

type fillHandle struct {
	value     string
	expires   time.Time
	sessionID string
	peer      string
}

// IssueFill creates an opaque, one-shot capability. The secret itself never
// leaves the daemon; only the handle is returned to the CLI.
func (e *Executor) IssueFill(cred *store.Credential, field, sessionID, peer string, ttl time.Duration) (string, time.Time, error) {
	value, err := fillValue(cred, field)
	if err != nil {
		return "", time.Time{}, err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	handle := base64.RawURLEncoding.EncodeToString(raw)
	if ttl <= 0 {
		ttl = session.DefaultFillTTL
	}
	expires := time.Now().Add(ttl)
	e.mu.Lock()
	for h, v := range e.fillCalls {
		if time.Now().After(v.expires) {
			delete(e.fillCalls, h)
		}
	}
	e.fillCalls[handle] = &fillHandle{value: value, expires: expires, sessionID: sessionID, peer: peer}
	e.mu.Unlock()
	return handle, expires, nil
}

func (e *Executor) RevokeFill(handle string) {
	e.mu.Lock()
	delete(e.fillCalls, handle)
	e.mu.Unlock()
}

// ApplyFill consumes a handle and injects its value into a child process, via
// both stdin and AGENT_PWD_FILL. Nothing prints the value.
func (e *Executor) ApplyFill(ctx context.Context, handle, sessionID, peer string, argv []string) (map[string]any, error) {
	if len(argv) == 0 {
		return nil, errors.New("executor: fill apply requires a command after --")
	}
	e.mu.Lock()
	f, ok := e.fillCalls[handle]
	if ok {
		delete(e.fillCalls, handle) // one shot, even if the child later fails
	}
	e.mu.Unlock()
	if !ok {
		return nil, errors.New("executor: unknown, used or expired fill handle")
	}
	if time.Now().After(f.expires) {
		return nil, errors.New("executor: fill handle expired")
	}
	if f.sessionID != "" && f.sessionID != sessionID {
		return nil, errors.New("executor: fill handle belongs to a different session")
	}
	if f.peer != "" && f.peer != peer {
		return nil, errors.New("executor: fill handle belongs to a different client")
	}
	red := NewRedactor(e.redactor.Secrets()...)
	red.Add(f.value)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), "AGENT_PWD_FILL="+f.value)
	cmd.Stdin = strings.NewReader(f.value + "\n")
	out, err := cmd.CombinedOutput()
	text, truncated := capString(string(red.RedactBytes(out)))
	exit := 0
	var ee *exec.ExitError
	if err != nil && errors.As(err, &ee) {
		exit = ee.ExitCode()
	} else if err != nil {
		return nil, fmt.Errorf("executor: run child: %w", err)
	}
	return map[string]any{"output": text, "exit_code": exit, "truncated": truncated}, nil
}

func fillValue(cred *store.Credential, field string) (string, error) {
	s := cred.Secret
	var value string
	switch cred.Kind {
	case store.KindWeb:
		switch field {
		case "password":
			value = s.Password
		case "totp":
			return totp(s.TOTPSeed)
		case "username":
			value = s.Username
		case "url":
			value = s.URL
		default:
			return "", fmt.Errorf("executor: unknown fill field %q", field)
		}
	case store.KindAPIKey:
		if field != "token" {
			return "", fmt.Errorf("executor: apikey supports only field=token")
		}
		value = s.Token
	case store.KindEnvVar:
		if field != "value" {
			return "", fmt.Errorf("executor: envvar supports only field=value")
		}
		value = s.Value
	case store.KindServer:
		if field != "password" {
			return "", fmt.Errorf("executor: server supports only field=password")
		}
		value = s.Password
	default:
		return "", errors.New("executor: unsupported credential kind for fill")
	}
	if value == "" {
		return "", errors.New("executor: credential has no value for that field")
	}
	return value, nil
}

func totp(seed string) (string, error) {
	seed = strings.ToUpper(strings.TrimSpace(strings.ReplaceAll(seed, " ", "")))
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(seed)
	if err != nil {
		key, err = base32.StdEncoding.DecodeString(seed)
		if err != nil {
			return "", fmt.Errorf("executor: invalid TOTP seed: %w", err)
		}
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(time.Now().Unix()/30))
	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", code%1000000), nil
}

