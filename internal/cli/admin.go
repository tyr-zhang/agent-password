package cli

import (
	"bufio"
	"io"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tyr-zhang/agent-password/internal/daemon"
	"github.com/tyr-zhang/agent-password/internal/platform"
	"github.com/tyr-zhang/agent-password/internal/policy"
	"github.com/tyr-zhang/agent-password/internal/store"
	"golang.org/x/term"
)

func init() {
	register(&Command{
		Name: "init", Group: "Management (human)", Syntax: "agent-pwd init",
		Summary: "initialize the vault and set the master password",
		Flags: []Flag{{Name: "password", Summary: "master password (prefer interactive prompt)"},
			{Name: "new-password", Summary: "same as --password"}},
		Doc: []string{"The master password is never stored. Losing it means losing the vault."},
		Run: cmdInit,
	})
	register(&Command{
		Name: "unlock", Group: "Management (human)", Syntax: "agent-pwd unlock",
		Summary: "unlock the session in the running daemon",
		Flags:   []Flag{{Name: "password", Summary: "master password (prefer interactive prompt)"}},
		Run: func(c *Context) int {
			p, err := readMaster(c, "password")
			if err != nil {
				return c.Fail(1, err.Error())
			}
			result, e := Call("unlock", map[string]any{"password": p})
			return emit(c, result, e)
		},
	})
	register(&Command{
		Name: "lock", Group: "Management (human)", Syntax: "agent-pwd lock",
		Summary: "zeroize session keys and invalidate fill handles/pending actions",
		Run:     simpleCall("lock"),
	})
	register(&Command{
		Name: "status", Group: "Management (human)", Syntax: "agent-pwd status",
		Summary: "show daemon, session and KDF status (no secrets)",
		Run:     simpleCall("status"),
	})
	register(&Command{
		Name: "cred add", Group: "Management (human)", Syntax: "agent-pwd cred add <web|apikey|server|envvar> <label>",
		Summary: "interactively add a credential; secret fields never enter shell history",
		Flags: []Flag{
			{Name: "secret", Summary: "optional JSON secret object (for automation only)"},
			{Name: "risk", Summary: "normal or high (high forces confirmation)"},
			{Name: "tags", Summary: "comma-separated tags"},
			{Name: "port", Summary: "server port (optional; overrides secret.port)"},
		},
		Doc: []string{
			"Without --secret, fields are read from the terminal/stdin; sensitive fields are hidden.",
			"Adding an existing label overwrites that credential; its ID and created_at are preserved.",
		},
		Run: cmdCredAdd,
	})
	register(&Command{
		Name: "cred list", Group: "Management (human)", Syntax: "agent-pwd cred list",
		Summary: "list credential metadata", Run: simpleCall("cred.list"),
	})
	register(&Command{
		Name: "cred show", Group: "Management (human)", Syntax: "agent-pwd cred show <id|label>",
		Summary: "show metadata only; there is no path that prints a secret",
		Run: func(c *Context) int {
			result, e := Call("cred.show", map[string]any{"id": c.Arg(0)})
			return emit(c, result, e)
		},
	})
	register(&Command{
		Name: "cred rm", Group: "Management (human)", Syntax: "agent-pwd cred rm <id|label>",
		Summary: "delete a credential",
		Run: func(c *Context) int {
			result, e := Call("cred.rm", map[string]any{"id": c.Arg(0)})
			return emit(c, result, e)
		},
	})
	register(&Command{
		Name: "cred restore", Group: "Management (human)", Syntax: "agent-pwd cred restore --from <backup>",
		Summary: "restore a vault backup after explicit human verification",
		Flags:   []Flag{{Name: "from", Summary: "path to vault.dat.bak.N"}},
		Run: func(c *Context) int {
			result, e := Call("cred.restore", map[string]any{"from": c.String("from", "")})
			return emit(c, result, e)
		},
	})
	register(&Command{
		Name: "policy add", Group: "Management (human)", Syntax: "agent-pwd policy add --cred-pattern '<glob>' --action '<glob>' --decision allow|confirm|deny",
		Summary: "insert a policy rule at the front (new rules have highest priority)",
		Flags: []Flag{
			{Name: "cred-pattern", Summary: `credential glob, e.g. "apikey:*"`},
			{Name: "action", Summary: `action glob: ssh|sql|api|fill|env|*`},
			{Name: "decision", Summary: "allow, confirm or deny"},
			{Name: "condition", Summary: "optional: sql_readonly or http_readonly"},
			{Name: "id", Summary: "optional rule id"},
		},
		Run: func(c *Context) int {
			if c.String("decision", "") == "" || !policy.ValidDecision(c.String("decision", "")) {
				return c.Fail(1, c.T("decision must be allow, confirm or deny"))
			}
			result, e := Call("policy.add", map[string]any{
				"cred_pattern": c.String("cred-pattern", ""), "action": c.String("action", ""),
				"decision": c.String("decision", ""), "condition": c.String("condition", ""),
				"id": c.String("id", ""),
			})
			return emit(c, result, e)
		},
	})
	register(&Command{Name: "policy list", Group: "Management (human)", Syntax: "agent-pwd policy list",
		Summary: "list rules in priority order", Run: simpleCall("policy.list")})
	register(&Command{Name: "policy rm", Group: "Management (human)", Syntax: "agent-pwd policy rm <id>",
		Summary: "remove a rule by id", Run: func(c *Context) int {
			result, e := Call("policy.rm", map[string]any{"id": c.Arg(0)})
			return emit(c, result, e)
		}})
	register(&Command{Name: "audit tail", Group: "Management (human)", Syntax: "agent-pwd audit tail [--n 50]",
		Summary: "show the latest audit records (no secret material)",
		Flags:   []Flag{{Name: "n", Summary: "number of records", Default: "50"}},
		Run: func(c *Context) int {
			result, e := Call("audit.tail", map[string]any{"n": c.Int("n", 50)})
			return emit(c, result, e)
		}})
	register(&Command{Name: "audit verify", Group: "Management (human)", Syntax: "agent-pwd audit verify",
		Summary: "verify the full audit hash chain", Run: simpleCall("audit.verify")})
	register(&Command{Name: "doctor", Group: "Management (human)", Syntax: "agent-pwd doctor",
		Summary: "check permissions, backups, audit chain, daemon and platform hardening", Run: simpleCall("doctor")})
	register(&Command{Name: "rotate-password", Group: "Management (human)", Syntax: "agent-pwd rotate-password [--new-salt]",
		Summary: "change the master password (rewrap DEK only unless --new-salt)",
		Flags: []Flag{{Name: "new-salt", Bool: true, Summary: "derive a fresh DEK and re-encrypt all credentials"},
			{Name: "password", Summary: "current master password (prefer interactive prompt)"},
			{Name: "new-password", Summary: "new password (prefer interactive prompt)"}},
		Run: cmdRotatePassword})
	register(&Command{Name: "service install", Group: "Management (human)", Syntax: "agent-pwd service install",
		Summary: "install a per-user LaunchAgent / systemd unit / scheduled task",
		Run: func(c *Context) int {
			exe, err := os.Executable()
			if err != nil {
				return c.Fail(1, err.Error())
			}
			if err := platform.Current().InstallService(exe); err != nil {
				return c.Fail(1, err.Error())
			}
			return c.PrintJSON(map[string]any{"status": "installed"})
		}})
	register(&Command{Name: "service uninstall", Group: "Management (human)", Syntax: "agent-pwd service uninstall",
		Summary: "remove the per-user autostart entry", Run: func(c *Context) int {
			if err := platform.Current().UninstallService(); err != nil {
				return c.Fail(1, err.Error())
			}
			return c.PrintJSON(map[string]any{"status": "uninstalled"})
		}})
	register(&Command{Name: "service status", Group: "Management (human)", Syntax: "agent-pwd service status",
		Summary: "report autostart status", Run: func(c *Context) int {
			s, err := platform.Current().ServiceStatus()
			if err != nil {
				return c.Fail(1, err.Error())
			}
			return emit(c, map[string]any{"status": strings.TrimSpace(s)}, nil)
		}})
	register(&Command{Name: "daemon", Group: "Internal", Syntax: "agent-pwd daemon [--foreground]",
		Hidden: true, Summary: "run the resident daemon (never started implicitly)",
		Flags:  []Flag{{Name: "foreground", Bool: true, Summary: "stay attached to the terminal"}},
		Run: func(c *Context) int {
			d, err := daemon.New(daemon.ConfigFromEnv())
			if err != nil {
				return c.Fail(1, err.Error())
			}
			if c.Bool("foreground") {
				ctx, stop := context.WithCancel(context.Background())
				defer stop()
				installSignals(ctx, stop)
				if err := d.Run(ctx); err != nil {
					c.Warn(err.Error())
					return 1
				}
				return 0
			}
			if err := detachDaemon(); err != nil {
				return c.Fail(1, err.Error())
			}
			return 0
		}})
}

func simpleCall(op string) func(*Context) int {
	return func(c *Context) int {
		result, e := Call(op, map[string]any{})
		return emit(c, result, e)
	}
}

func cmdInit(c *Context) int {
	pass, err := readMaster(c, "new-password")
	if err != nil {
		return c.Fail(1, err.Error())
	}
	if c.String("password", "") == "" && c.String("new-password", "") == "" {
		again, err := readLine(c, c.T("confirm master password")+": ", true)
		if err != nil || again != pass {
			return c.Fail(1, c.T("passwords do not match"))
		}
	}
	dir, err := platform.DataDir()
	if err != nil {
		return c.Fail(1, err.Error())
	}
	s := store.New(filepath.Join(dir, "vault.dat"))
	body := &store.Body{Schema: 1, UpdatedAt: time.Now().UTC(),
		Credentials: []store.Credential{}, Policies: policy.DefaultRules()}
	if err := s.Create(pass, body); err != nil {
		return c.Fail(1, err.Error())
	}
	return c.PrintJSON(map[string]any{"status": "initialized", "vault": s.Path(),
		"policies": len(body.Policies), "next": c.T("start the daemon, then agent-pwd unlock")})
}

func cmdRotatePassword(c *Context) int {
	old, err := readMaster(c, "password")
	if err != nil {
		return c.Fail(1, err.Error())
	}
	next := c.String("new-password", "")
	if next == "" {
		var err error
		next, err = readLine(c, c.T("new master password")+": ", true)
		if err != nil || next == "" {
			return c.Fail(1, c.T("new password is empty or unreadable"))
		}
		again, err := readLine(c, c.T("confirm new master password")+": ", true)
		if err != nil || again != next {
			return c.Fail(1, c.T("passwords do not match"))
		}
	}
	result, e := Call("auth.rotate-password", map[string]any{
		"old_password": old, "new_password": next, "new_salt": c.Bool("new-salt"),
	})
	return emit(c, result, e)
}

var stdinReader = bufio.NewReader(os.Stdin)

func readMaster(c *Context, flag string) (string, error) {
	if v := c.String(flag, ""); v != "" {
		return v, nil
	}
	if v := c.String("password", ""); v != "" {
		return v, nil
	}
	return readLine(c, c.T("master password")+": ", true)
}

func readLine(c *Context, prompt string, hidden bool) (string, error) {
	fmt.Fprint(c.Stderr, prompt)
	if hidden && term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(c.Stderr)
		return string(b), err
	}
	line, err := stdinReader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	if line == "" {
		return "", fmt.Errorf("%s", c.T("no input available; use a terminal or --password"))
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func cmdCredAdd(c *Context) int {
	kind, label := c.Arg(0), c.Arg(1)
	if !store.ValidKind(store.Kind(kind)) || label == "" {
		return c.Fail(1, c.T("usage: cred add <web|apikey|server|envvar> <label>"))
	}
	var secret store.Secret
	if raw := c.String("secret", ""); raw != "" {
		if err := json.Unmarshal([]byte(raw), &secret); err != nil {
			return c.Fail(1, c.T("invalid --secret JSON: %s", err.Error()))
		}
	} else {
		var err error
		secret, err = promptSecret(c, store.Kind(kind))
		if err != nil {
			return c.Fail(1, err.Error())
		}
	}
	result, e := Call("cred.add", map[string]any{
		"kind": kind, "label": label, "secret": secret,
		"risk_level": c.String("risk", "normal"), "tags": c.Strings("tags"),
	})
	return emit(c, result, e)
}

func promptSecret(c *Context, kind store.Kind) (store.Secret, error) {
	var s store.Secret
	var portText string
	fields := []struct {
		name   string
		target *string
		hidden bool
	}{}
	switch kind {
	case store.KindWeb:
		fields = []struct {
			name   string
			target *string
			hidden bool
		}{{"username", &s.Username, false}, {"password", &s.Password, true},
			{"totp_seed (optional)", &s.TOTPSeed, false}, {"url (optional)", &s.URL, false}}
	case store.KindAPIKey:
		fields = []struct {
			name   string
			target *string
			hidden bool
		}{{"base_url", &s.BaseURL, false}, {"header_name (default Authorization)", &s.HeaderName, false},
			{"token", &s.Token, true}}
	case store.KindServer:
		fields = []struct {
			name   string
			target *string
			hidden bool
		}{{"protocol (ssh/mysql/postgres/redis)", &s.Protocol, false}, {"host", &s.Host, false},
			{"port (optional)", &portText, false},
			{"server_user", &s.ServerUser, false},
			{"password (or leave empty for key)", &s.Password, true},
			{"private_key (optional)", &s.PrivateKey, true},
			{"key_path (optional)", &s.KeyPath, false},
			{"passphrase (optional)", &s.Passphrase, true},
			{"database (optional)", &s.Database, false}}
	case store.KindEnvVar:
		fields = []struct {
			name   string
			target *string
			hidden bool
		}{{"key", &s.Key, false}, {"value", &s.Value, true}}
	}
	for _, f := range fields {
		v, err := readLine(c, c.T(f.name)+": ", f.hidden)
		if err != nil {
			return s, err
		}
		*f.target = v
	}
	if n := parsePort(portText); n > 0 {
		s.Port = n
	}
	// Backward-compatible alias: --port overrides secret.port.
	if v := c.Int("port", 0); v > 0 {
		s.Port = v
	}
	return s, nil
}

func parsePort(text string) int {
	n, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || n <= 0 || n > 65535 {
		return 0
	}
	return n
}
