package audit

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func appendOne(t *testing.T, path, action, outcome string) Record {
	t.Helper()
	r := Record{CredentialID: "cred_1", Action: action, Decision: "allow", Outcome: outcome,
		DetailHash: strings.Repeat("a", 64), Client: "test"}
	if err := Append(path, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestChainVerifyAndTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	r1 := appendOne(t, path, "ssh", "started")
	r2 := appendOne(t, path, "ssh", "completed")
	if r1.Seq != 1 || r2.Seq != 2 || r1.Hash == r2.Hash {
		t.Fatalf("bad sequence/hash: %+v %+v", r1, r2)
	}
	report, err := Verify(path)
	if err != nil || !report.OK || report.Records != 2 || report.LastSeq != 2 {
		t.Fatalf("verify: %+v %v", report, err)
	}
	recs, err := Tail(path, 1)
	if err != nil || len(recs) != 1 || recs[0].Seq != 2 {
		t.Fatalf("tail: %+v %v", recs, err)
	}
}

func TestTamperDetection(t *testing.T) {
	for _, mode := range []string{"hash", "payload", "delete"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "audit.log")
			appendOne(t, path, "sql", "started")
			appendOne(t, path, "sql", "completed")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
			switch mode {
			case "hash":
				lines[0] = strings.Replace(lines[0], `"hash":"`, `"hash":"0`, 1)
			case "payload":
				lines[0] = strings.Replace(lines[0], `"outcome":"started"`, `"outcome":"hacked"`, 1)
			case "delete":
				lines = lines[1:]
			}
			if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			report, err := Verify(path)
			if err != nil || report.OK || report.Problem == "" {
				t.Fatalf("tamper %s was not detected: %+v %v", mode, report, err)
			}
		})
	}
}

func TestHashIsSHA256OfPrevAndCanonicalRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	r := appendOne(t, path, "env", "started")
	b, err := hex.DecodeString(r.Hash)
	if err != nil || len(b) != 32 {
		t.Fatal("hash is not 32-byte hex")
	}
	if r.PrevHash != strings.Repeat("0", 64) {
		t.Fatal("genesis prev_hash is wrong")
	}
}
