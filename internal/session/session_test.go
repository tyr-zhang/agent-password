package session

import (
	"bytes"
	"testing"
	"time"

	"github.com/tyr-zhang/agent-password/internal/crypto"
)

func TestUnlockKeysProofAndLock(t *testing.T) {
	s := New(DefaultConfig())
	if s.IsUnlocked() {
		t.Fatal("new session is unlocked")
	}
	dek := bytes.Repeat([]byte{1}, 32)
	kek := bytes.Repeat([]byte{2}, 32)
	s.Unlock(dek, kek)
	gotDEK, gotKEK, err := s.Keys()
	if err != nil || !bytes.Equal(gotDEK, dek) || !bytes.Equal(gotKEK, kek) {
		t.Fatalf("keys: %v", err)
	}
	proof := crypto.Proof(kek, "c_1", "nonce")
	if !s.VerifyProof("c_1", "nonce", proof) {
		t.Fatal("proof rejected")
	}
	s.Lock()
	if _, _, err := s.Keys(); err == nil {
		t.Fatal("locked session returned keys")
	}
	if s.VerifyProof("c_1", "nonce", proof) {
		t.Fatal("locked session verified a proof")
	}
}

func TestBackoffAndStop(t *testing.T) {
	s := New(DefaultConfig())
	var stop bool
	for i, want := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second} {
		got, shouldStop := s.Failure()
		if got != want {
			t.Errorf("failure %d: backoff %v, want %v", i+1, got, want)
		}
		stop = shouldStop
	}
	if !stop {
		t.Fatal("five failures did not request shutdown")
	}
	s.ResetFailures()
	if s.BackoffRemaining() != 0 {
		t.Fatal("reset did not clear backoff")
	}
}

func TestIdleExpiry(t *testing.T) {
	s := New(Config{IdleTimeout: 20 * time.Millisecond, HardTimeout: time.Hour})
	s.Unlock(bytes.Repeat([]byte{3}, 32), bytes.Repeat([]byte{4}, 32))
	time.Sleep(40 * time.Millisecond)
	if _, _, err := s.Keys(); err == nil {
		t.Fatal("idle session did not expire")
	}
}

func TestDefaultTimeouts(t *testing.T) {
	if DefaultIdleTimeout != 8*time.Hour || DefaultHardTimeout != 24*time.Hour {
		t.Fatalf("unexpected default session timeouts: idle=%v hard=%v", DefaultIdleTimeout, DefaultHardTimeout)
	}
}
