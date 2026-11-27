package daemon

import (
	"crypto/hmac"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tyr-zhang/agent-password/internal/audit"
	"github.com/tyr-zhang/agent-password/internal/crypto"
	"github.com/tyr-zhang/agent-password/internal/i18n"
	"github.com/tyr-zhang/agent-password/internal/policy"
	"github.com/tyr-zhang/agent-password/internal/platform"
	"github.com/tyr-zhang/agent-password/internal/protocol"
	"github.com/tyr-zhang/agent-password/internal/store"
)

func argString(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatInt(int64(x), 10)
	case bool:
		return strconv.FormatBool(x)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func argBool(args map[string]any, key string) bool {
	b, _ := args[key].(bool)
	return b
}

func argInt(args map[string]any, key string, def int) int {
	s := argString(args, key)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func argStrings(args map[string]any, key string) []string {
	raw, ok := args[key]
	if !ok {
		return nil
	}
	switch x := raw.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, v := range x {
			out = append(out, argString(map[string]any{"v": v}, "v"))
		}
		return out
	default:
		if s := argString(args, key); s != "" {
			return []string{s}
		}
		return nil
	}
}

func argMap(args map[string]any, key string) map[string]any {
	m, _ := args[key].(map[string]any)
	return m
}

// secretsOf collects every secret string in the vault so reflected values
// from any endpoint are scrubbed, not just the credential in use.
func secretsOf(body *store.Body) []string {
	var out []string
	for i := range body.Credentials {
		v := reflect.ValueOf(body.Credentials[i].Secret)
		for j := 0; j < v.NumField(); j++ {
			f := v.Field(j)
			if f.Kind() == reflect.String && f.String() != "" {
				out = append(out, f.String())
			}
		}
	}
	return out
}

func newID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return prefix + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return prefix + hex.EncodeToString(b)
}

func (d *Daemon) handleAdmin(req protocol.Request) protocol.Response {
	switch req.Op {
	case "lock":
		d.sess.Lock()
		d.store.Lock()
		d.pending.cancelAll()
		d.redactor.Reset()
		return protocol.OK(map[string]any{"status": "locked"})
	case "cred.add":
		return d.credAdd(req)
	case "cred.list":
		body, err := d.store.Body()
		if err != nil {
			return protocol.Fail(protocol.CodeInternal, err.Error())
		}
		pub := make([]store.PublicCredential, 0, len(body.Credentials))
		for i := range body.Credentials {
			pub = append(pub, store.Public(&body.Credentials[i]))
		}
		return protocol.OK(map[string]any{"credentials": pub})
	case "cred.show":
		body, err := d.store.Body()
		if err != nil {
			return protocol.Fail(protocol.CodeInternal, err.Error())
		}
		id := argString(req.Args, "id")
		for i := range body.Credentials {
			if body.Credentials[i].ID == id {
				return protocol.OK(map[string]any{"credential": store.Public(&body.Credentials[i])})
			}
		}
		return protocol.Fail(protocol.CodeNotFound, "credential not found")
	case "cred.rm":
		body, err := d.store.Body()
		if err != nil {
			return protocol.Fail(protocol.CodeInternal, err.Error())
		}
		id := argString(req.Args, "id")
		idx := -1
		for i := range body.Credentials {
			if body.Credentials[i].ID == id {
				idx = i
				break
			}
		}
		if idx < 0 {
			return protocol.Fail(protocol.CodeNotFound, "credential not found")
		}
		body.Credentials = append(body.Credentials[:idx], body.Credentials[idx+1:]...)
		if err := d.store.Save(body); err != nil {
			return protocol.Fail(protocol.CodeInternal, err.Error())
		}
		d.redactor.Reset(secretsOf(body)...)
		return protocol.OK(map[string]any{"status": "removed", "id": id})
	case "policy.add":
		body, err := d.store.Body()
		if err != nil {
			return protocol.Fail(protocol.CodeInternal, err.Error())
		}
		r := store.Rule{
			ID:        argString(req.Args, "id"),
			CredPat:   argString(req.Args, "cred_pattern"),
			Action:    argString(req.Args, "action"),
			Condition: argString(req.Args, "condition"),
			Decision:  argString(req.Args, "decision"),
		}
		if r.ID == "" {
			r.ID = newID("rule_")
		}
		if r.CredPat == "" || r.Action == "" || !policy.ValidDecision(r.Decision) || !policy.ValidCondition(r.Condition) {
			return protocol.Fail(protocol.CodeBadArgs, "invalid rule (need cred_pattern, action, decision, optional condition)")
		}
		for _, existing := range body.Policies {
			if existing.ID == r.ID {
				return protocol.Fail(protocol.CodeExists, "rule id already exists")
			}
		}
		body.Policies = append([]store.Rule{r}, body.Policies...)
		if err := d.store.Save(body); err != nil {
			return protocol.Fail(protocol.CodeInternal, err.Error())
		}
		return protocol.OK(map[string]any{"rule": r})
	case "policy.list":
		body, err := d.store.Body()
		if err != nil {
			return protocol.Fail(protocol.CodeInternal, err.Error())
		}
		return protocol.OK(map[string]any{"policies": body.Policies})
	case "policy.rm":
		body, err := d.store.Body()
		if err != nil {
			return protocol.Fail(protocol.CodeInternal, err.Error())
		}
		id := argString(req.Args, "id")
		idx := -1
		for i := range body.Policies {
			if body.Policies[i].ID == id {
				idx = i
				break
			}
		}
		if idx < 0 {
			return protocol.Fail(protocol.CodeNotFound, "rule not found")
		}
		body.Policies = append(body.Policies[:idx], body.Policies[idx+1:]...)
		if err := d.store.Save(body); err != nil {
			return protocol.Fail(protocol.CodeInternal, err.Error())
		}
		return protocol.OK(map[string]any{"status": "removed", "id": id})
	case "audit.tail":
		recs, err := audit.Tail(d.auditPath, argInt(req.Args, "n", 50))
		if err != nil {
			return protocol.Fail(protocol.CodeInternal, err.Error())
		}
		return protocol.OK(map[string]any{"records": recs})
	case "auth.rotate-password":
		return d.rotatePassword(req)
	default:
		return protocol.Fail(protocol.CodeBadArgs, "unknown operation")
	}
}

func (d *Daemon) rotatePassword(req protocol.Request) protocol.Response {
	old := argString(req.Args, "old_password")
	newPass := argString(req.Args, "new_password")
	if old == "" || newPass == "" {
		return protocol.Fail(protocol.CodeBadArgs, "old_password and new_password are required")
	}
	h := d.store.Header()
	if h == nil {
		return protocol.Fail(protocol.CodeLocked, "vault is locked")
	}
	kek, err := crypto.DeriveKEK(old, h.Salt, h.KDF)
	if err != nil {
		return protocol.Fail(protocol.CodeInternal, err.Error())
	}
	current := d.store.KEK()
	ok := hmac.Equal(kek, current)
	crypto.Zero(kek)
	crypto.Zero(current)
	if !ok {
		retry, stop := d.sess.Failure()
		if stop {
			go d.Stop()
			return protocol.FailRetry(protocol.CodeInternal, "wrong password; too many failed attempts, daemon stopping", int(retry.Seconds()))
		}
		return protocol.FailRetry(protocol.CodeLocked, "wrong master password", int(retry.Seconds()))
	}
	body, err := d.store.Body()
	if err != nil {
		return protocol.Fail(protocol.CodeInternal, err.Error())
	}
	if err := d.store.RotatePassword(newPass, argBool(req.Args, "new_salt")); err != nil {
		return protocol.Fail(protocol.CodeInternal, err.Error())
	}
	dek, freshKEK := d.store.DEKKEK()
	d.sess.Unlock(dek, freshKEK)
	crypto.Zero(dek)
	crypto.Zero(freshKEK)
	d.redactor.Reset(secretsOf(body)...)
	return protocol.OK(map[string]any{"status": "password rotated"})
}

func (d *Daemon) credAdd(req protocol.Request) protocol.Response {
	body, err := d.store.Body()
	if err != nil {
		return protocol.Fail(protocol.CodeInternal, err.Error())
	}
	kind := store.Kind(argString(req.Args, "kind"))
	label := argString(req.Args, "label")
	if !store.ValidKind(kind) || label == "" {
		return protocol.Fail(protocol.CodeBadArgs, "valid kind and label are required")
	}
	var s store.Secret
	raw, _ := json.Marshal(argMap(req.Args, "secret"))
	if len(raw) > 2 {
		if err := json.Unmarshal(raw, &s); err != nil {
			return protocol.Fail(protocol.CodeBadArgs, "invalid secret: "+err.Error())
		}
	}
	if err := validateSecret(kind, &s); err != nil {
		return protocol.Fail(protocol.CodeBadArgs, err.Error())
	}
	risk := argString(req.Args, "risk_level")
	if risk == "" {
		risk = "normal"
	}
	if risk != "normal" && risk != "high" {
		return protocol.Fail(protocol.CodeBadArgs, "risk_level must be normal or high")
	}
	now := time.Now()
	c := store.Credential{
		ID: newID("cred_"), Kind: kind, Label: label, RiskLevel: risk,
		Tags: argStrings(req.Args, "tags"), CreatedAt: now, UpdatedAt: now, Secret: s,
	}
	body.Credentials = append(body.Credentials, c)
	if err := d.store.Save(body); err != nil {
		return protocol.Fail(protocol.CodeInternal, err.Error())
	}
	d.redactor.Reset(secretsOf(body)...)
	return protocol.OK(map[string]any{"credential": store.Public(&c)})
}

func validateSecret(kind store.Kind, s *store.Secret) error {
	switch kind {
	case store.KindWeb:
		if s.Username == "" || s.Password == "" {
			return fmt.Errorf("web requires username and password")
		}
	case store.KindAPIKey:
		if s.BaseURL == "" || s.Token == "" {
			return fmt.Errorf("apikey requires base_url and token")
		}
		if s.HeaderName == "" {
			s.HeaderName = "Authorization"
		}
	case store.KindServer:
		switch s.Protocol {
		case "ssh", "mysql", "postgres", "postgresql", "redis":
		default:
			return fmt.Errorf("server protocol must be ssh, mysql, postgres or redis")
		}
		if s.Host == "" || s.ServerUser == "" {
			return fmt.Errorf("server requires host and server_user")
		}
		if s.Password == "" && s.PrivateKey == "" && s.KeyPath == "" {
			return fmt.Errorf("server requires password or private key")
		}
	case store.KindEnvVar:
		if s.Key == "" || s.Value == "" {
			return fmt.Errorf("envvar requires key and value")
		}
	}
	return nil
}

func (d *Daemon) doctor(req protocol.Request) protocol.Response {
	st, err := os.Stat(d.dataDir)
	perms := "unknown"
	if err == nil {
		perms = fmt.Sprintf("%#o", st.Mode().Perm())
	}
	backups := d.store.Backups()
	var newest string
	if len(backups) > 0 {
		sort.Slice(backups, func(i, j int) bool { return backups[i].ModTime.After(backups[j].ModTime) })
		newest = backups[0].ModTime.Format(time.RFC3339)
	}
	report, _ := audit.Verify(d.auditPath)
	d.mu.RLock()
	broken := d.auditBroken
	d.mu.RUnlock()
	return protocol.OK(map[string]any{
		"data_dir": d.dataDir, "dir_permissions": perms,
		"vault_exists": store.Exists(d.store.Path()),
		"audit_ok": report.OK && broken == nil, "audit_records": report.Records,
		"audit_problem": strings.Join(nonEmpty(report.Problem, errText(broken)), "; "),
		"backups": len(backups), "newest_backup": newest,
		"daemon": "running", "service": platformServiceStatus(),
		"platform": platform.Current().Name(),
		"security_note": i18n.T(i18n.ParseLang(req.Lang), platform.Current().SecurityNote()),
		"unlocked": d.sess.IsUnlocked(),
	})
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func nonEmpty(values ...string) []string {
	var out []string
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

