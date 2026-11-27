// Package audit implements the append-only, hash-chained action log. Every
// record links to the previous hash so deletion and rewriting are detectable.
package audit

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

var ErrChainBroken = errors.New("audit: hash chain broken")

type Record struct {
	Seq           uint64 `json:"seq"`
	TS            string `json:"ts"`
	CredentialID  string `json:"credential_id"`
	Action        string `json:"action"`
	Decision      string `json:"decision"`
	ConfirmSource string `json:"confirm_source"`
	Outcome       string `json:"outcome"`
	ResultCode    string `json:"result_code"`
	DetailHash    string `json:"detail_hash"`
	PolicyRule    string `json:"policy_rule"`
	Client        string `json:"client"`
	PrevHash      string `json:"prev_hash"`
	Hash          string `json:"hash"`
}

func hashOf(prev []byte, r Record) []byte {
	r.Hash = ""
	canon, _ := json.Marshal(r) // struct field order is deterministic
	h := sha256.New()
	h.Write(prev)
	h.Write(canon)
	return h.Sum(nil)
}

func genesis() []byte { return make([]byte, sha256.Size) }

// Append writes one record with an exclusive lock and fsync before returning.
func Append(path string, r *Record) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("audit: open log: %w", err)
	}
	defer f.Close()
	if err := lockFile(f); err != nil {
		return err
	}
	defer unlockFile(f)

	last, seq, err := lastHash(path)
	if err != nil {
		return err
	}
	r.Seq = seq + 1
	if r.TS == "" {
		r.TS = time.Now().UTC().Format(time.RFC3339Nano)
	}
	r.PrevHash = hex.EncodeToString(last)
	r.Hash = hex.EncodeToString(hashOf(last, *r))
	line, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("audit: append: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("audit: fsync: %w", err)
	}
	return nil
}

func lastHash(path string) ([]byte, uint64, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return genesis(), 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	var last []byte
	var seq uint64
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, 0, fmt.Errorf("audit: malformed record: %w", err)
		}
		b, err := hex.DecodeString(rec.Hash)
		if err != nil || len(b) != sha256.Size {
			return nil, 0, ErrChainBroken
		}
		last, seq = b, rec.Seq
	}
	if err := sc.Err(); err != nil {
		return nil, 0, err
	}
	if last == nil {
		return genesis(), 0, nil
	}
	return last, seq, nil
}

type VerifyReport struct {
	OK       bool   `json:"ok"`
	Records  int    `json:"records"`
	FirstSeq uint64 `json:"first_seq,omitempty"`
	LastSeq  uint64 `json:"last_seq,omitempty"`
	Problem  string `json:"problem,omitempty"`
}

// Verify walks the whole chain and detects broken hashes, gaps, duplicate or
// out-of-order sequence numbers, and truncated lines.
func Verify(path string) (VerifyReport, error) {
	out := VerifyReport{OK: true}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	defer f.Close()
	prev := genesis()
	var want uint64 = 1
	var lastTS time.Time
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return fail(out, fmt.Sprintf("line %d: malformed JSON", lineNo)), nil
		}
		if rec.Seq != want {
			return fail(out, fmt.Sprintf("line %d: expected seq %d, got %d", lineNo, want, rec.Seq)), nil
		}
		ph, err := hex.DecodeString(rec.PrevHash)
		if err != nil || len(ph) != sha256.Size || !equalBytes(ph, prev) {
			return fail(out, fmt.Sprintf("line %d: prev_hash mismatch", lineNo)), nil
		}
		got, err := hex.DecodeString(rec.Hash)
		if err != nil || !equalBytes(got, hashOf(prev, rec)) {
			return fail(out, fmt.Sprintf("line %d: hash mismatch", lineNo)), nil
		}
		ts, err := time.Parse(time.RFC3339Nano, rec.TS)
		if err != nil {
			return fail(out, fmt.Sprintf("line %d: bad timestamp", lineNo)), nil
		}
		if ts.Before(lastTS) {
			return fail(out, fmt.Sprintf("line %d: non-monotonic timestamp", lineNo)), nil
		}
		lastTS = ts
		if out.Records == 0 {
			out.FirstSeq = rec.Seq
		}
		out.Records++
		out.LastSeq = rec.Seq
		prev, want = got, rec.Seq+1
	}
	if err := sc.Err(); err != nil {
		return fail(out, err.Error()), nil
	}
	return out, nil
}

func fail(r VerifyReport, why string) VerifyReport {
	r.OK = false
	r.Problem = why
	return r
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

// Tail returns the last n records in chronological order.
func Tail(path string, n int) ([]Record, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []Record{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var all []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("audit: malformed record: %w", err)
		}
		all = append(all, rec)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if n > 0 && len(all) > n {
		all = all[len(all)-n:]
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Seq < all[j].Seq })
	return all, nil
}
