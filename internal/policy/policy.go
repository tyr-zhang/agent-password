// Package policy evaluates ordered rules and classifies write vs read
// operations. Unknown actions always fall through to confirm.
package policy

import (
	"path"
	"regexp"
	"strings"

	"github.com/tyr-zhang/agent-password/internal/store"
)

const (
	Allow   = "allow"
	Confirm = "confirm"
	Deny    = "deny"
)

type Decision struct {
	Decision string `json:"decision"`
	RuleID   string `json:"rule_id"`
	Reason   string `json:"reason"`
}

type Context struct {
	Method string // api
	SQL    string // sql / redis
}

func DefaultRules() []store.Rule {
	return []store.Rule{
		{ID: "default-ssh", CredPat: "server:*", Action: "ssh", Condition: "", Decision: Allow},
		{ID: "default-sql-read", CredPat: "server:*", Action: "sql", Condition: "sql_readonly", Decision: Allow},
		{ID: "default-api-read", CredPat: "apikey:*", Action: "api", Condition: "http_readonly", Decision: Allow},
		{ID: "default-fill", CredPat: "*", Action: "fill", Condition: "", Decision: Allow},
		{ID: "default-env", CredPat: "*", Action: "env", Condition: "", Decision: Allow},
		{ID: "default-sql-write", CredPat: "*", Action: "sql", Condition: "", Decision: Confirm},
		{ID: "default-api-write", CredPat: "*", Action: "api", Condition: "", Decision: Confirm},
		{ID: "default-fallback", CredPat: "*", Action: "*", Condition: "", Decision: Confirm},
	}
}

func ValidDecision(d string) bool { return d == Allow || d == Confirm || d == Deny }

func ValidCondition(c string) bool { return c == "" || c == "sql_readonly" || c == "http_readonly" }

func matches(pattern, value string) bool {
	ok, err := path.Match(pattern, value)
	return err == nil && ok
}

// Decide applies the first matching rule. A high-risk credential can downgrade
// an allow to confirm, but an explicit deny always wins.
func Decide(rules []store.Rule, cred *store.Credential, action string, ctx Context) Decision {
	target := string(cred.Kind) + ":" + cred.ID
	for _, r := range rules {
		if !matches(r.CredPat, target) || !matches(r.Action, action) {
			continue
		}
		switch r.Condition {
		case "":
		case "sql_readonly":
			if !SQLReadOnly(ctx.SQL) {
				continue
			}
		case "http_readonly":
			if !HTTPReadOnly(ctx.Method) {
				continue
			}
		default:
			continue
		}
		d := r.Decision
		if d == Allow && cred.RiskLevel == "high" {
			d = Confirm
		}
		return Decision{Decision: d, RuleID: r.ID, Reason: "rule:" + r.ID}
	}
	return Decision{Decision: Confirm, RuleID: "fallback", Reason: "unmatched"}
}

var sqlWord = regexp.MustCompile(`^[A-Za-z]+`)

// SQLReadOnly rejects multi-statement input and comment tricks, and only
// permits the four explicitly allow-listed leading verbs. WITH is treated as
// a write conservatively.
func SQLReadOnly(q string) bool {
	s := stripComments(q)
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if strings.Contains(s, ";") {
		trimmed := strings.TrimSuffix(s, ";")
		if strings.Contains(trimmed, ";") {
			return false
		}
		s = strings.TrimSpace(trimmed)
	}
	m := sqlWord.FindString(s)
	if m == "" {
		return false
	}
	switch strings.ToUpper(m) {
	case "SELECT", "SHOW", "DESCRIBE", "DESC", "EXPLAIN":
		return true
	default:
		return false
	}
}

func stripComments(s string) string {
	var b strings.Builder
	inLine, inBlock := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inLine {
			if c == '\n' {
				inLine = false
				b.WriteByte(c)
			}
			continue
		}
		if inBlock {
			if c == '*' && i+1 < len(s) && s[i+1] == '/' {
				inBlock = false
				i++
			}
			continue
		}
		if c == '-' && i+1 < len(s) && s[i+1] == '-' {
			inLine = true
			i++
			continue
		}
		if c == '/' && i+1 < len(s) && s[i+1] == '*' {
			inBlock = true
			i++
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func HTTPReadOnly(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "GET", "HEAD":
		return true
	default:
		return false
	}
}
