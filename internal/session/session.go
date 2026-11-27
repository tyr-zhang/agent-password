// Package session owns the unlocked state: the in-memory DEK/KEK, idle and
// hard timeouts, and the failed-unlock backoff required by the design.
package session

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/tyr-zhang/agent-password/internal/crypto"
)

const (
	DefaultIdleTimeout  = 8 * time.Hour
	DefaultHardTimeout  = 24 * time.Hour
	DefaultConfirmTTL   = 5 * time.Minute
	DefaultFillTTL      = 60 * time.Second
	MaxFailedAttempts   = 5
	MaxBackoff          = 60 * time.Second
)

var ErrLocked = errors.New("session: vault is locked")

type Config struct {
	IdleTimeout time.Duration
	HardTimeout time.Duration
	ConfirmTTL  time.Duration
	FillTTL     time.Duration
}

func DefaultConfig() Config {
	return Config{
		IdleTimeout: DefaultIdleTimeout,
		HardTimeout: DefaultHardTimeout,
		ConfirmTTL:  DefaultConfirmTTL,
		FillTTL:     DefaultFillTTL,
	}
}

type Session struct {
	mu         sync.Mutex
	cfg        Config
	unlockedAt time.Time
	lastUse    time.Time
	dek        []byte
	kek        []byte
	failures   int
	lockUntil  time.Time
	onLock     func()
}

func New(cfg Config) *Session {
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = DefaultIdleTimeout
	}
	if cfg.HardTimeout <= 0 {
		cfg.HardTimeout = DefaultHardTimeout
	}
	return &Session{cfg: cfg}
}

// OnLock registers a callback invoked whenever the session transitions to
// locked (including idle expiry detected during an action).
func (s *Session) OnLock(fn func()) { s.onLock = fn }

func (s *Session) Unlock(dek, kek []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	crypto.Zero(s.dek)
	crypto.Zero(s.kek)
	s.dek = append([]byte(nil), dek...)
	s.kek = append([]byte(nil), kek...)
	s.failures = 0
	s.lockUntil = time.Time{}
	s.unlockedAt = time.Now()
	s.lastUse = s.unlockedAt
}

func (s *Session) Lock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clear()
}

func (s *Session) clear() {
	crypto.Zero(s.dek)
	crypto.Zero(s.kek)
	s.dek = nil
	s.kek = nil
	s.unlockedAt = time.Time{}
	s.lastUse = time.Time{}
	fn := s.onLock
	if fn != nil {
		go fn()
	}
}

func (s *Session) IsUnlocked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked() == nil
}

func (s *Session) statusLocked() error {
	if s.dek == nil || s.kek == nil {
		return ErrLocked
	}
	if !s.lockUntil.IsZero() && time.Now().After(s.lockUntil) {
		s.clear()
		return ErrLocked
	}
	if time.Since(s.unlockedAt) > s.cfg.HardTimeout || time.Since(s.lastUse) > s.cfg.IdleTimeout {
		s.clear()
		return ErrLocked
	}
	return nil
}

// Keys returns copies of the session keys, enforcing timeouts.
func (s *Session) Keys() (dek, kek []byte, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.statusLocked(); err != nil {
		return nil, nil, err
	}
	s.lastUse = time.Now()
	return append([]byte(nil), s.dek...), append([]byte(nil), s.kek...), nil
}

// Touch records a successful action, preventing idle lockout.
func (s *Session) Touch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dek != nil {
		s.lastUse = time.Now()
	}
}

func (s *Session) Failure() (retryAfter time.Duration, shouldStop bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures++
	d := time.Second
	for i := 1; i < s.failures && d < MaxBackoff; i++ {
		d *= 2
	}
	if d > MaxBackoff {
		d = MaxBackoff
	}
	s.lockUntil = time.Now().Add(d)
	return d, s.failures >= MaxFailedAttempts
}

// BackoffRemaining reports how long the caller should wait after failed
// unlock attempts. Zero means another attempt is allowed now.
func (s *Session) BackoffRemaining() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lockUntil.IsZero() {
		return 0
	}
	d := time.Until(s.lockUntil)
	if d < 0 {
		return 0
	}
	return d
}

func (s *Session) ResetFailures() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = 0
	s.lockUntil = time.Time{}
}

// VerifyProof checks the HMAC proof computed by the confirming client from
// the master password. The password itself never reaches the daemon.
func (s *Session) VerifyProof(id, nonce, proof string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.statusLocked(); err != nil {
		return false
	}
	return crypto.VerifyProof(s.kek, id, nonce, proof)
}

func (s *Session) Status() (unlocked bool, expiresAt time.Time, failures int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.statusLocked(); err != nil {
		return false, time.Time{}, s.failures
	}
	idle := s.lastUse.Add(s.cfg.IdleTimeout)
	hard := s.unlockedAt.Add(s.cfg.HardTimeout)
	if hard.Before(idle) {
		idle = hard
	}
	return true, idle, s.failures
}

// StableID derives a short, non-secret identifier for this unlocked session,
// used to bind fill handles to a session generation.
func (s *Session) StableID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dek == nil {
		return ""
	}
	sum := sha256.Sum256(s.dek)
	return hex.EncodeToString(sum[:8])
}
