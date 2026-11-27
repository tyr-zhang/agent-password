package executor

import (
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"testing"
)

func TestRedactorScrubsEncodedCanaries(t *testing.T) {
	secret := "canary-secret-abcdefgh"
	r := NewRedactor(secret)
	cases := []string{
		"token=" + secret,
		"Bearer " + secret,
		hex.EncodeToString([]byte(secret)),
		base64.StdEncoding.EncodeToString([]byte(secret)),
		url.QueryEscape(secret),
		`"token":"` + secret + `"`,
	}
	for _, input := range cases {
		out := r.Redact("prefix " + input + " suffix")
		if out == input || contains(out, secret) {
			t.Errorf("canary survived redaction: %q -> %q", input, out)
		}
	}
	if got := r.Redact("ordinary output"); got != "ordinary output" {
		t.Fatalf("ordinary output changed: %q", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestRedactorGenericPatterns(t *testing.T) {
	r := NewRedactor()
	out := r.Redact("Authorization: Bearer abcdefghijklmnop api_key = shhhhhhhhhh")
	if contains(out, "abcdefghijklmnop") || contains(out, "shhhhhhhhhh") {
		t.Fatalf("generic secrets survived: %q", out)
	}
}
