package daemon

import (
	"testing"

	"github.com/tyr-zhang/agent-password/internal/store"
)

func TestFindCredentialIndex(t *testing.T) {
	body := &store.Body{Credentials: []store.Credential{
		{ID: "cred_a", Label: "deploy"},
		{ID: "cred_b", Label: "backup"},
	}}
	cases := map[string]int{"deploy": 0, "backup": 1, "cred_a": 0, "cred_b": 1, "missing": -1}
	for ref, want := range cases {
		if got := findCredentialIndex(body, ref); got != want {
			t.Errorf("findCredentialIndex(%q)=%d want %d", ref, got, want)
		}
	}
}

func TestLabelWinsOverAnotherCredentialsID(t *testing.T) {
	body := &store.Body{Credentials: []store.Credential{
		{ID: "cred_a", Label: "cred_b"},
		{ID: "cred_b", Label: "backup"},
	}}
	if got := findCredentialIndex(body, "cred_b"); got != 0 {
		t.Fatalf("label must win over ID: got index %d", got)
	}
}
