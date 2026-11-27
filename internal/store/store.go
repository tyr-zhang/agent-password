package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tyr-zhang/agent-password/internal/crypto"
)

const DefaultBackups = 5

var (
	ErrNotInitialized = errors.New("store: vault does not exist; run agent-pwd init")
	ErrAlreadyExists  = errors.New("store: vault already initialized")
	ErrLocked         = errors.New("store: vault is locked")
)

// BackupInfo describes a backup candidate without exposing any secret.
type BackupInfo struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

type Store struct {
	mu      sync.Mutex
	path    string
	backups int
	image   []byte
	header  *crypto.Header
	dek     []byte
	kek     []byte
	body    *Body
}

func New(path string) *Store {
	return &Store{path: path, backups: DefaultBackups}
}

func (s *Store) Path() string { return s.path }

func Exists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".vault-*.tmp")
	if err != nil {
		return fmt.Errorf("store: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("store: write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("store: fsync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("store: rename vault: %w", err)
	}
	df, err := os.Open(dir)
	if err == nil {
		_ = df.Sync()
		_ = df.Close()
	}
	return nil
}

func (s *Store) Create(password string, body *Body) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if Exists(s.path) {
		return ErrAlreadyExists
	}
	plain, err := json.Marshal(body)
	if err != nil {
		return err
	}
	image, dek, kek, err := crypto.Create(password, plain)
	if err != nil {
		return err
	}
	if err := atomicWrite(s.path, image); err != nil {
		crypto.Zero(dek)
		crypto.Zero(kek)
		return err
	}
	s.image = image
	s.dek = dek
	s.kek = kek
	s.header, err = crypto.ParseHeader(image)
	if err != nil {
		return err
	}
	s.body, err = cloneBody(body)
	return err
}

// Unlock verifies the password, decrypts the body and keeps the DEK/KEK in
// memory for the lifetime of the session.
func (s *Store) Unlock(password string) (*Body, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	image, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotInitialized
		}
		return nil, err
	}
	dek, kek, plain, header, err := crypto.Open(password, image)
	if err != nil {
		return nil, err
	}
	var body Body
	if err := json.Unmarshal(plain, &body); err != nil {
		crypto.Zero(dek)
		crypto.Zero(kek)
		return nil, fmt.Errorf("store: decode vault body: %w", err)
	}
	if body.Schema == 0 {
		body.Schema = 1
	}
	s.Lock_memory()
	s.image = image
	s.dek = dek
	s.kek = kek
	s.header = header
	s.body = &body
	return &body, nil
}

// Lock_memory releases any previously held keys. The exported method name
// avoids confusion with the CLI's `lock` operation.
func (s *Store) Lock_memory() {
	crypto.Zero(s.dek)
	crypto.Zero(s.kek)
	s.dek = nil
	s.kek = nil
	s.header = nil
	s.body = nil
}

func (s *Store) Lock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Lock_memory()
}

func (s *Store) IsUnlocked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dek != nil && s.body != nil
}

func cloneBody(b *Body) (*Body, error) {
	plain, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	var out Body
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Body returns a deep copy so callers cannot mutate the in-memory vault
// without going through Save.
func (s *Store) Body() (*Body, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.body == nil {
		return nil, ErrLocked
	}
	return cloneBody(s.body)
}

func (s *Store) Header() *crypto.Header {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.header == nil {
		return nil
	}
	h := *s.header
	h.Prefix = append([]byte(nil), s.header.Prefix...)
	return &h
}

func (s *Store) KEK() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.kek...)
}

// DEKKEK returns copies for the daemon's session so that Store and Session
// can independently zero their own buffers on lock.
func (s *Store) DEKKEK() (dek, kek []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.dek...), append([]byte(nil), s.kek...)
}

func (s *Store) Save(body *Body) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dek == nil {
		return ErrLocked
	}
	plain, err := json.Marshal(body)
	if err != nil {
		return err
	}
	image, err := crypto.ReplaceBody(s.image, s.dek, plain)
	if err != nil {
		return err
	}
	if err := s.rotateBackups(); err != nil {
		return err
	}
	if err := atomicWrite(s.path, image); err != nil {
		return err
	}
	s.image = image
	h, err := crypto.ParseHeader(image)
	if err != nil {
		return err
	}
	s.header = h
	s.body, err = cloneBody(body)
	return err
}

func (s *Store) RotatePassword(newPassword string, newSalt bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dek == nil || s.kek == nil || s.body == nil {
		return ErrLocked
	}
	plain, err := json.Marshal(s.body)
	if err != nil {
		return err
	}
	image, dek, kek, err := crypto.Rewrap(s.image, s.kek, s.dek, newPassword, plain, newSalt)
	if err != nil {
		return err
	}
	if err := s.rotateBackups(); err != nil {
		crypto.Zero(dek)
		crypto.Zero(kek)
		return err
	}
	if err := atomicWrite(s.path, image); err != nil {
		crypto.Zero(dek)
		crypto.Zero(kek)
		return err
	}
	crypto.Zero(s.dek)
	crypto.Zero(s.kek)
	s.image, s.dek, s.kek = image, dek, kek
	s.header, err = crypto.ParseHeader(image)
	return err
}

func (s *Store) backupPath(n int) string {
	return fmt.Sprintf("%s.bak.%d", s.path, n)
}

func (s *Store) rotateBackups() error {
	if !Exists(s.path) {
		return nil
	}
	for i := s.backups - 1; i >= 1; i-- {
		src := s.backupPath(i)
		if !Exists(src) {
			continue
		}
		if err := os.Rename(src, s.backupPath(i+1)); err != nil {
			return fmt.Errorf("store: rotate backup %s: %w", src, err)
		}
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	return atomicWrite(s.backupPath(1), data)
}

func (s *Store) Backups() []BackupInfo {
	var out []BackupInfo
	for i := 1; i <= s.backups; i++ {
		p := s.backupPath(i)
		st, err := os.Stat(p)
		if err != nil || st.IsDir() {
			continue
		}
		out = append(out, BackupInfo{Path: p, Size: st.Size(), ModTime: st.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return strings.Compare(out[i].Path, out[j].Path) < 0 })
	return out
}

// RestoreFrom copies a backup over the vault file. It requires an explicit
// human command and never decrypts or prints secret material.
func (s *Store) RestoreFrom(backup string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !Exists(backup) {
		return os.ErrNotExist
	}
	data, err := os.ReadFile(backup)
	if err != nil {
		return err
	}
	if _, err := crypto.ParseHeader(data); err != nil {
		return fmt.Errorf("store: not a valid vault backup: %w", err)
	}
	if err := s.rotateBackups(); err != nil {
		return err
	}
	if err := atomicWrite(s.path, data); err != nil {
		return err
	}
	s.Lock_memory()
	s.image = nil
	return nil
}
