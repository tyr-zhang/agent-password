package policy

import (
	"testing"

	"github.com/tyr-zhang/agent-password/internal/store"
)

func TestDefaultDecisionTable(t *testing.T) {
	rules := DefaultRules()
	cases := []struct {
		kind   store.Kind
		action string
		ctx    Context
		want   string
	}{
		{store.KindServer, "ssh", Context{}, Allow},
		{store.KindServer, "sql", Context{SQL: "SELECT 1"}, Allow},
		{store.KindServer, "sql", Context{SQL: "SHOW TABLES"}, Allow},
		{store.KindServer, "sql", Context{SQL: "DESCRIBE users"}, Allow},
		{store.KindServer, "sql", Context{SQL: "INSERT INTO t VALUES (1)"}, Confirm},
		{store.KindServer, "sql", Context{SQL: "WITH x AS (SELECT 1) SELECT * FROM x"}, Confirm},
		{store.KindAPIKey, "api", Context{Method: "GET"}, Allow},
		{store.KindAPIKey, "api", Context{Method: "HEAD"}, Allow},
		{store.KindAPIKey, "api", Context{Method: "POST"}, Confirm},
		{store.KindWeb, "fill", Context{}, Allow},
		{store.KindEnvVar, "env", Context{}, Allow},
		{store.KindWeb, "ssh", Context{}, Confirm},
	}
	for _, tc := range cases {
		c := &store.Credential{ID: "1", Kind: tc.kind, RiskLevel: "normal"}
		got := Decide(rules, c, tc.action, tc.ctx)
		if got.Decision != tc.want {
			t.Errorf("%s/%s %+v: got %s want %s", tc.kind, tc.action, tc.ctx, got.Decision, tc.want)
		}
	}
}

func TestHighRiskAndDenyPrecedence(t *testing.T) {
	rules := DefaultRules()
	high := &store.Credential{ID: "1", Kind: store.KindServer, RiskLevel: "high"}
	if got := Decide(rules, high, "ssh", Context{}); got.Decision != Confirm {
		t.Fatalf("high-risk ssh: %+v", got)
	}
	deny := append([]store.Rule{{ID: "explicit", CredPat: "*", Action: "*", Decision: Deny}}, rules...)
	if got := Decide(deny, &store.Credential{ID: "1", Kind: store.KindServer}, "ssh", Context{}); got.Decision != Deny {
		t.Fatalf("explicit deny: %+v", got)
	}
}

func TestSQLReadOnlyTricks(t *testing.T) {
	cases := map[string]bool{
		"SELECT 1":                                true,
		"  /* comment */ SELECT 1":                true,
		"SELECT 1;":                               true,
		"SELECT 1; DROP TABLE x":                  false,
		"-- SELECT 1\nDELETE FROM x":              false,
		"/* SELECT */ UPDATE x SET y=1":           false,
		"WITH cte AS (SELECT 1) SELECT * FROM cte": false,
		"": false,
	}
	for q, want := range cases {
		if got := SQLReadOnly(q); got != want {
			t.Errorf("SQLReadOnly(%q)=%v want %v", q, got, want)
		}
	}
}
