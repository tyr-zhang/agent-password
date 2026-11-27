// Package executor runs credential-backed actions. It is the only place that
// turns stored secrets into usable material, and every outbound string is
// passed through the redactor before it can reach the CLI.
package executor

import (
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var (
	secretPattern = regexp.MustCompile(`(?i)(authorization|proxy-authorization|api[_-]?key|apikey|token|password|passwd|secret|credential)\s*[:=]\s*("[^"]*"|'[^']*'|\S+)`)
	bearerPattern = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]{8,}`)
)

type Redactor struct {
	mu      sync.RWMutex
	secrets []string
}

func NewRedactor(secrets ...string) *Redactor {
	return &Redactor{secrets: append([]string(nil), secrets...)}
}

// Add registers a secret. Short values are ignored because replacing them
// would destroy most ordinary output without meaningful protection.
func (r *Redactor) Add(s string) {
	if len(s) < 4 {
		return
	}
	r.mu.Lock()
	r.secrets = append(r.secrets, s)
	r.mu.Unlock()
}

// Reset replaces the secret set, e.g. after unlocking or changing a vault.
func (r *Redactor) Reset(secrets ...string) {
	r.mu.Lock()
	r.secrets = append([]string(nil), secrets...)
	r.mu.Unlock()
}

func (r *Redactor) Secrets() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]string(nil), r.secrets...)
}

func (r *Redactor) variants() []string {
	r.mu.RLock()
	src := append([]string(nil), r.secrets...)
	r.mu.RUnlock()
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if len(s) >= 4 && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range src {
		add(s)
		add(url.QueryEscape(s))
		add(hex.EncodeToString([]byte(s)))
		add(base64.StdEncoding.EncodeToString([]byte(s)))
		add(base64.RawURLEncoding.EncodeToString([]byte(s)))
		add(strconv.Quote(s))
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

func (r *Redactor) Redact(s string) string {
	if s == "" {
		return s
	}
	for _, v := range r.variants() {
		s = strings.ReplaceAll(s, v, "[REDACTED]")
	}
	s = bearerPattern.ReplaceAllString(s, "Bearer [REDACTED]")
	s = secretPattern.ReplaceAllString(s, "$1=[REDACTED]")
	return s
}

func (r *Redactor) RedactBytes(b []byte) []byte {
	return []byte(r.Redact(string(b)))
}
