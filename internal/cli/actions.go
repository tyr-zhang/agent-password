package cli

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/tyr-zhang/agent-password/internal/crypto"
	"github.com/tyr-zhang/agent-password/internal/i18n"
	"github.com/tyr-zhang/agent-password/internal/platform"
	"github.com/tyr-zhang/agent-password/internal/protocol"
)

func init() {
	register(&Command{
		Name: "ssh exec", Group: "Actions (AI)", Syntax: "agent-pwd ssh exec --cred <label|id> -- <command...>",
		Summary: "run a command over SSH using the stored credential, return redacted stdout",
		Flags: []Flag{{Name: "cred", Summary: "credential label or id"},
			{Name: "insecure-host-key", Bool: true, Summary: "allow unknown host keys (use only for tests)"}},
		Doc: []string{"Passwords/private keys are never returned. Strict known_hosts verification is the default."},
		Run: func(c *Context) int {
			cmd := strings.Join(c.ArgsFrom(0), " ")
			result, e := Call("action.ssh", map[string]any{
				"cred": c.String("cred", ""), "command": cmd,
				"insecure_host_key": c.Bool("insecure-host-key"),
			})
			return emitAction(c, result, e)
		},
	})
	register(&Command{
		Name: "sql query", Group: "Actions (AI)", Syntax: `agent-pwd sql query --cred <label|id> -- "<SQL>" [--arg v]...`,
		Summary: "execute SQL (mysql/postgres) or a read-only redis command",
		Flags:   []Flag{{Name: "cred", Summary: "credential label or id"}, {Name: "arg", Summary: "bind parameter (repeatable)"}},
		Doc:     []string{"SELECT/SHOW/DESCRIBE/EXPLAIN are auto-allowed; writes require confirmation."},
		Run: func(c *Context) int {
			result, e := Call("action.sql", map[string]any{
				"cred": c.String("cred", ""), "query": c.Arg(0), "args": c.Strings("arg"),
			})
			return emitAction(c, result, e)
		},
	})
	register(&Command{
		Name: "api call", Group: "Actions (AI)", Syntax: "agent-pwd api call --cred <label|id> --method GET --path /v1/x",
		Summary: "make an HTTP request with the stored token injected by the daemon",
		Flags: []Flag{{Name: "cred", Summary: "apikey credential label or id"},
			{Name: "method", Summary: "HTTP method", Default: "GET"},
			{Name: "path", Summary: "absolute path appended to base_url"},
			{Name: "body", Summary: "request body"},
			{Name: "header", Summary: "Extra header k:v (repeatable; cannot override auth/cookie)"},
			{Name: "allow-redirect", Bool: true, Summary: "allow same-origin redirects (off by default)"}},
		Run: func(c *Context) int {
			headers := map[string]any{}
			for _, h := range c.Strings("header") {
				parts := strings.SplitN(h, ":", 2)
				if len(parts) != 2 {
					return c.Fail(1, c.T("header must be k:v"))
				}
				headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
			result, e := Call("action.api", map[string]any{
				"cred": c.String("cred", ""), "method": c.String("method", "GET"),
				"path": c.String("path", ""), "body": c.String("body", ""),
				"headers": headers, "allow_redirect": c.Bool("allow-redirect"),
			})
			return emitAction(c, result, e)
		},
	})
	register(&Command{
		Name: "env run", Group: "Actions (AI)", Syntax: "agent-pwd env run --cred <label|id> -- <command...>",
		Summary: "inject an envvar credential into a child process and return redacted output",
		Flags:   []Flag{{Name: "cred", Summary: "envvar credential label or id"}},
		Run: func(c *Context) int {
			result, e := Call("action.env", map[string]any{
				"cred": c.String("cred", ""), "argv": c.ArgsFrom(0),
			})
			return emitAction(c, result, e)
		},
	})
	register(&Command{
		Name: "fill", Group: "Actions (AI)", Syntax: "agent-pwd fill --cred <label|id> --field password|totp|username|token",
		Summary: "issue a one-shot, expiring handle; never returns the secret itself",
		Flags:   []Flag{{Name: "cred", Summary: "credential label or id"},
			{Name: "field", Summary: "field to inject", Default: "password"}},
		Doc: []string{"Consume with `fill apply --handle <handle> -- <command>`. The handle expires in 60 seconds."},
		Run: func(c *Context) int {
			result, e := Call("fill.issue", map[string]any{
				"cred": c.String("cred", ""), "field": c.String("field", "password"),
			})
			return emitAction(c, result, e)
		},
	})
	register(&Command{
		Name: "fill apply", Group: "Actions (AI)", Syntax: "agent-pwd fill apply --handle <h> -- <command...>",
		Summary: "consume a fill handle and inject the value into a child process via stdin/env",
		Flags:   []Flag{{Name: "handle", Summary: "opaque handle from `fill`"}},
		Run: func(c *Context) int {
			result, e := Call("fill.apply", map[string]any{
				"handle": c.String("handle", ""), "argv": c.ArgsFrom(0),
			})
			return emitAction(c, result, e)
		},
	})
	register(&Command{
		Name: "confirm approve", Group: "Confirmation", Syntax: "agent-pwd confirm approve <id> [--no-biometric]",
		Summary: "approve a pending action via Touch ID / Windows Hello, falling back to master password",
		Flags: []Flag{{Name: "no-biometric", Bool: true, Summary: "skip the native biometric attempt"},
			{Name: "password", Summary: "master password for proof (prefer interactive prompt)"}},
		Run: cmdConfirmApprove,
	})
	register(&Command{
		Name: "confirm deny", Group: "Confirmation", Syntax: "agent-pwd confirm deny <id>",
		Summary: "reject a pending action", Run: func(c *Context) int {
			result, e := Call("confirm.deny", map[string]any{"id": c.Arg(0)})
			return emit(c, result, e)
		},
	})
	register(&Command{
		Name: "confirm result", Group: "Confirmation", Syntax: "agent-pwd confirm result <id> --token <poll_token> [--wait]",
		Summary: "fetch the outcome of a confirmed action (one-shot final result)",
		Flags: []Flag{{Name: "token", Summary: "poll token from the exit-10 response"},
			{Name: "wait", Bool: true, Summary: "poll until the decision is made"}},
		Run: cmdConfirmResult,
	})
}

func emitAction(c *Context, result map[string]any, e *CallError) int {
	if e != nil {
		return emit(c, result, e)
	}
	if result["status"] == "pending" {
		code := emit(c, result, nil)
		if c.Bool("wait") {
			id, _ := result["confirmation_id"].(string)
			token, _ := result["poll_token"].(string)
			return waitForResult(c, id, token)
		}
		if code == 0 {
			return protocol.ExitCode(protocol.CodePending)
		}
		return code
	}
	return emit(c, result, nil)
}

func waitForResult(c *Context, id, token string) int {
	deadline := time.Now().Add(6 * time.Minute)
	for time.Now().Before(deadline) {
		result, e := Call("confirm.result", map[string]any{"id": id, "token": token})
		if e != nil {
			return emit(c, result, e)
		}
		if result["status"] != "pending" {
			return emit(c, result, nil)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return c.Fail(1, c.T("timed out waiting for confirmation"))
}

func cmdConfirmResult(c *Context) int {
	id, token := c.Arg(0), c.String("token", "")
	if id == "" || token == "" {
		return c.Fail(1, c.T("usage: confirm result <id> --token <poll_token>"))
	}
	if !c.Bool("wait") {
		result, e := Call("confirm.result", map[string]any{"id": id, "token": token})
		return emit(c, result, e)
	}
	return waitForResult(c, id, token)
}

func cmdConfirmApprove(c *Context) int {
	id := c.Arg(0)
	if id == "" {
		return c.Fail(1, c.T("usage: confirm approve <id>"))
	}
	if !c.Bool("no-biometric") && platform.Current().BiometricAvailable() {
		result, e := Call("confirm.approve", map[string]any{"id": id, "biometric": true})
		if e == nil {
			return emit(c, result, nil)
		}
		if e.Code != protocol.CodeBiometric {
			return emit(c, result, e)
		}
		c.Warn(c.T("biometric confirmation unavailable; falling back to master password"))
	}
	info, e := Call("confirm.info", map[string]any{"id": id})
	if e != nil {
		return emit(c, info, e)
	}
	password, err := readMaster(c, "password")
	if err != nil {
		return c.Fail(1, err.Error())
	}
	kek, err := deriveKEKFromStatus(password)
	if err != nil {
		return c.Fail(1, err.Error())
	}
	nonce, _ := info["nonce"].(string)
	proof := crypto.Proof(kek, id, nonce)
	crypto.Zero(kek)
	result, e := Call("confirm.approve", map[string]any{"id": id, "proof": proof})
	return emit(c, result, e)
}

func deriveKEKFromStatus(password string) ([]byte, error) {
	status, e := Call("status", map[string]any{})
	if e != nil {
		return nil, e
	}
	kdf, ok := status["kdf"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s", i18n.T(currentLang, "status response has no KDF parameters"))
	}
	saltText, _ := kdf["salt"].(string)
	saltBytes, err := base64.StdEncoding.DecodeString(saltText)
	if err != nil || len(saltBytes) != crypto.SaltLen {
		return nil, fmt.Errorf("%s", i18n.T(currentLang, "invalid salt in status response"))
	}
	var salt [crypto.SaltLen]byte
	copy(salt[:], saltBytes)
	p := crypto.KDFParams{
		Time:        uint32(numberValue(kdf["time"])),
		MemoryKiB:   uint32(numberValue(kdf["memory_kib"])),
		Parallelism: uint8(numberValue(kdf["parallelism"])),
	}
	return crypto.DeriveKEK(password, salt, p)
}

func numberValue(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	default:
		return 0
	}
}
