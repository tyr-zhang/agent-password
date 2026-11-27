package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testBody() *Body {
	return &Body{
		Schema: 1,
		Credentials: []Credential{{
			ID: "cred_1", Kind: KindWeb, Label: "example", RiskLevel: "normal",
			Secret: Secret{Username: "alice", Password: "canary-secret-1111"},
		}},
		Policies: []Rule{{ID: "r1", CredPat: "*", Action: "*", Decision: "confirm"}},
	}
}

func TestCreateSaveBackupAndRestore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.dat")
	s := New(path)
	if err := s.Create("pw", testBody()); err != nil {
		t.Fatal(err)
	}
	if !Exists(path) {
		t.Fatal("vault missing")
	}
	if err := s.Create("pw", testBody()); err != ErrAlreadyExists {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}
	body, err := s.Unlock("pw")
	if err != nil {
		t.Fatal(err)
	}
	if len(body.Credentials) != 1 {
		t.Fatal("unexpected body")
	}
	body.Credentials[0].Label = "changed"
	if err := s.Save(body); err != nil {
		t.Fatal(err)
	}
	backups := s.Backups()
	if len(backups) < 1 {
		t.Fatal("backup not created")
	}
	latest := backups[len(backups)-1].Path
	if err := s.RestoreFrom(latest); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if s.IsUnlocked() {
		t.Fatal("restore should lock the session")
	}
	after, err := s.Unlock("pw")
	if err != nil {
		t.Fatal(err)
	}
	if after.Credentials[0].Label != "example" {
		t.Fatalf("restore did not roll back: %q", after.Credentials[0].Label)
	}
}

func TestPublicCredentialHasNoSecret(t *testing.T) {
	c := testBody().Credentials[0]
	b, err := json.Marshal(Public(&c))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "canary-secret") || strings.Contains(string(b), "secret") {
		t.Fatalf("public credential leaked secret fields: %s", b)
	}
}

func TestWrongPasswordDoesNotDecrypt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.dat")
	s := New(path)
	if err := s.Create("right", testBody()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Unlock("wrong"); err == nil {
		t.Fatal("wrong password accepted")
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatalf("vault permissions: %v %v", st, err)
	}
}
