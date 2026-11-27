// Package crypto implements the vault's key derivation and authenticated
// encryption. No secret is ever written to disk in plaintext.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
)

const (
	Version       = 1
	KeyLen        = 32
	NonceLen      = 12
	SaltLen       = 32
	VaultIDLen    = 16
	WrappedDEKLen = KeyLen + 16 // GCM tag
	headerPrefix  = 68
	headerLen     = headerPrefix + NonceLen + 2 + WrappedDEKLen + NonceLen

	DefaultTime       = 3
	DefaultMemoryKiB  = 64 * 1024
	DefaultParallelism = 4
)

var (
	magic = [8]byte{'P', 'V', 'A', 'U', 'L', 'T', 0, 0}

	ErrBadMagic    = errors.New("crypto: not an agent-pwd vault")
	ErrBadVersion  = errors.New("crypto: unsupported vault version")
	ErrBadPassword = errors.New("crypto: wrong master password")
	ErrCorrupt     = errors.New("crypto: vault is corrupt or tampered with")
	ErrBadKDF      = errors.New("crypto: invalid KDF parameters")
)

// KDFParams are stored in the vault header so that changing them never
// requires a format change.
type KDFParams struct {
	Time       uint32
	MemoryKiB  uint32
	Parallelism uint8
}

// DefaultKDF returns the parameters fixed for new vaults.
func DefaultKDF() KDFParams {
	return KDFParams{Time: DefaultTime, MemoryKiB: DefaultMemoryKiB, Parallelism: DefaultParallelism}
}

// Header is the plaintext, non-secret part of the vault file. Everything
// mutable (the wrapped DEK and the body nonce) sits after Prefix.
type Header struct {
	Version     uint8
	VaultID     [VaultIDLen]byte
	KDF         KDFParams
	Salt        [SaltLen]byte
	DEKNonce    [NonceLen]byte
	WrappedDEK  [WrappedDEKLen]byte
	BodyNonce   [NonceLen]byte
	Prefix      []byte
}

// Zero overwrites a byte slice before releasing it. Go cannot guarantee this
// happens (copies may exist), so this is best-effort hardening.
func Zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("crypto: random source: %w", err)
	}
	return b, nil
}

func DeriveKEK(password string, salt [SaltLen]byte, p KDFParams) ([]byte, error) {
	if p.Time == 0 || p.Time > 100 || p.MemoryKiB < 8 || p.MemoryKiB > 512<<10 || p.Parallelism == 0 {
		return nil, ErrBadKDF
	}
	return argon2.IDKey([]byte(password), salt[:], p.Time, p.MemoryKiB, p.Parallelism, KeyLen), nil
}

func aad(prefix []byte, label string) []byte {
	out := make([]byte, 0, len(prefix)+len(label))
	out = append(out, prefix...)
	out = append(out, label...)
	return out
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func serializeHeader(h *Header, wrapped []byte, bodyNonce [NonceLen]byte) ([]byte, error) {
	if len(wrapped) != WrappedDEKLen {
		return nil, errors.New("crypto: bad wrapped DEK length")
	}
	out := make([]byte, 0, headerLen)
	out = append(out, magic[:]...)
	out = append(out, h.Version, 0)
	out = append(out, h.VaultID[:]...)
	var u32 [4]byte
	binary.BigEndian.PutUint32(u32[:], h.KDF.Time)
	out = append(out, u32[:]...)
	binary.BigEndian.PutUint32(u32[:], h.KDF.MemoryKiB)
	out = append(out, u32[:]...)
	out = append(out, h.KDF.Parallelism, 0)
	out = append(out, h.Salt[:]...)
	if len(out) != headerPrefix {
		return nil, errors.New("crypto: internal header layout error")
	}
	out = append(out, h.DEKNonce[:]...)
	var l [2]byte
	binary.BigEndian.PutUint16(l[:], uint16(len(wrapped)))
	out = append(out, l[:]...)
	out = append(out, wrapped...)
	out = append(out, bodyNonce[:]...)
	return out, nil
}

// ParseHeader validates the fixed-size parts of an image and returns its
// header plus a copy of the immutable prefix used as AEAD associated data.
func ParseHeader(image []byte) (*Header, error) {
	if len(image) < headerLen {
		return nil, ErrCorrupt
	}
	if string(image[:8]) != string(magic[:]) {
		return nil, ErrBadMagic
	}
	if image[8] > Version || image[8] == 0 {
		return nil, ErrBadVersion
	}
	h := &Header{
		Version: image[8],
		KDF: KDFParams{
			Time:        binary.BigEndian.Uint32(image[26:30]),
			MemoryKiB:   binary.BigEndian.Uint32(image[30:34]),
			Parallelism: image[34],
		},
		Prefix: append([]byte(nil), image[:headerPrefix]...),
	}
	copy(h.VaultID[:], image[10:26])
	copy(h.Salt[:], image[36:68])
	copy(h.DEKNonce[:], image[68:80])
	n := binary.BigEndian.Uint16(image[80:82])
	if int(n) != WrappedDEKLen || len(image) < 82+int(n)+NonceLen {
		return nil, ErrCorrupt
	}
	copy(h.WrappedDEK[:], image[82:82+int(n)])
	copy(h.BodyNonce[:], image[82+int(n):82+int(n)+NonceLen])
	if _, err := DeriveKEK("x", h.Salt, h.KDF); err != nil {
		return nil, err
	}
	return h, nil
}

func sealBody(dek []byte, prefix []byte, plaintext []byte) (nonce [NonceLen]byte, out []byte, err error) {
	raw, err := randomBytes(NonceLen)
	if err != nil {
		return nonce, nil, err
	}
	copy(nonce[:], raw)
	g, err := newGCM(dek)
	if err != nil {
		return nonce, nil, err
	}
	return nonce, g.Seal(nil, nonce[:], plaintext, aad(prefix, "/body/v1")), nil
}

// Create builds a new vault image. It returns the DEK and KEK so the caller
// can keep them in the unlocked session without deriving the KEK twice.
func Create(password string, plaintextBody []byte) (image, dek, kek []byte, err error) {
	return CreateWithParams(password, plaintextBody, DefaultKDF())
}

// CreateWithParams exists for tests and for users on memory-constrained
// machines; new vaults always use DefaultKDF.
func CreateWithParams(password string, plaintextBody []byte, p KDFParams) (image, dek, kek []byte, err error) {
	if password == "" {
		return nil, nil, nil, errors.New("crypto: empty master password")
	}
	if _, err := DeriveKEK(password, [SaltLen]byte{}, p); err != nil {
		return nil, nil, nil, err
	}
	h := &Header{Version: Version, KDF: p}
	id, err := randomBytes(VaultIDLen)
	if err != nil {
		return nil, nil, nil, err
	}
	copy(h.VaultID[:], id)
	salt, err := randomBytes(SaltLen)
	if err != nil {
		return nil, nil, nil, err
	}
	copy(h.Salt[:], salt)
	dek, err = randomBytes(KeyLen)
	if err != nil {
		return nil, nil, nil, err
	}
	kek, err = DeriveKEK(password, h.Salt, p)
	if err != nil {
		return nil, nil, nil, err
	}
	dNonce, err := randomBytes(NonceLen)
	if err != nil {
		return nil, nil, nil, err
	}
	copy(h.DEKNonce[:], dNonce)
	g, err := newGCM(kek)
	if err != nil {
		return nil, nil, nil, err
	}
	h.Prefix = append([]byte(nil), imagePrefix(h)...)
	wrapped := g.Seal(nil, h.DEKNonce[:], dek, aad(h.Prefix, "/dek/v1"))
	if len(wrapped) != WrappedDEKLen {
		return nil, nil, nil, errors.New("crypto: internal wrap error")
	}
	copy(h.WrappedDEK[:], wrapped)
	bodyNonce, bodyCT, err := sealBody(dek, h.Prefix, plaintextBody)
	if err != nil {
		return nil, nil, nil, err
	}
	image, err = serializeHeader(h, wrapped, bodyNonce)
	if err != nil {
		return nil, nil, nil, err
	}
	return append(image, bodyCT...), dek, kek, nil
}

// imagePrefix serializes just the immutable part of a header.
func imagePrefix(h *Header) []byte {
	out := make([]byte, 0, headerPrefix)
	out = append(out, magic[:]...)
	out = append(out, h.Version, 0)
	out = append(out, h.VaultID[:]...)
	var u32 [4]byte
	binary.BigEndian.PutUint32(u32[:], h.KDF.Time)
	out = append(out, u32[:]...)
	binary.BigEndian.PutUint32(u32[:], h.KDF.MemoryKiB)
	out = append(out, u32[:]...)
	out = append(out, h.KDF.Parallelism, 0)
	out = append(out, h.Salt[:]...)
	return out
}

// Open derives the KEK, unwraps the DEK and decrypts the body.
func Open(password string, image []byte) (dek, kek, body []byte, h *Header, err error) {
	h, err = ParseHeader(image)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	kek, err = DeriveKEK(password, h.Salt, h.KDF)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	g, err := newGCM(kek)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	dek, err = g.Open(nil, h.DEKNonce[:], h.WrappedDEK[:], aad(h.Prefix, "/dek/v1"))
	if err != nil {
		Zero(kek)
		return nil, nil, nil, nil, ErrBadPassword
	}
	body, err = OpenBody(dek, h.Prefix, h.BodyNonce, image[headerLen:])
	if err != nil {
		Zero(dek)
		Zero(kek)
		return nil, nil, nil, nil, err
	}
	return dek, kek, body, h, nil
}

// OpenBody decrypts a vault body with the given DEK and header prefix.
func OpenBody(dek, prefix []byte, nonce [NonceLen]byte, ct []byte) ([]byte, error) {
	g, err := newGCM(dek)
	if err != nil {
		return nil, err
	}
	plain, err := g.Open(nil, nonce[:], ct, aad(prefix, "/body/v1"))
	if err != nil {
		return nil, ErrCorrupt
	}
	return plain, nil
}

// ReplaceBody encrypts a new body under the same DEK. Only the body nonce in
// the mutable header section changes; the AAD prefix stays identical.
func ReplaceBody(image, dek, plaintext []byte) ([]byte, error) {
	h, err := ParseHeader(image)
	if err != nil {
		return nil, err
	}
	nonce, ct, err := sealBody(dek, h.Prefix, plaintext)
	if err != nil {
		return nil, err
	}
	out, err := serializeHeader(h, h.WrappedDEK[:], nonce)
	if err != nil {
		return nil, err
	}
	return append(out, ct...), nil
}

// Rewrap changes the master password without re-encrypting credentials when
// newSalt is false. With newSalt, a fresh DEK and body ciphertext are made.
func Rewrap(image []byte, oldKEK, dek []byte, newPassword string, newBody []byte, newSalt bool) (newImage, newDEK, newKEK []byte, err error) {
	h, err := ParseHeader(image)
	if err != nil {
		return nil, nil, nil, err
	}
	if passwordMatches(oldKEK, h, dek) == false {
		return nil, nil, nil, ErrBadPassword
	}
	if newPassword == "" {
		return nil, nil, nil, errors.New("crypto: empty master password")
	}
	if !newSalt {
		kek, err := DeriveKEK(newPassword, h.Salt, h.KDF)
		if err != nil {
			return nil, nil, nil, err
		}
		g, err := newGCM(kek)
		if err != nil {
			return nil, nil, nil, err
		}
		wrapped := g.Seal(nil, h.DEKNonce[:], dek, aad(h.Prefix, "/dek/v1"))
		out, err := serializeHeader(h, wrapped, h.BodyNonce)
		if err != nil {
			Zero(kek)
			return nil, nil, nil, err
		}
		return append(out, image[headerLen:]...), dek, kek, nil
	}
	salt, err := randomBytes(SaltLen)
	if err != nil {
		return nil, nil, nil, err
	}
	copy(h.Salt[:], salt)
	ndek, err := randomBytes(KeyLen)
	if err != nil {
		return nil, nil, nil, err
	}
	kek, err := DeriveKEK(newPassword, h.Salt, h.KDF)
	if err != nil {
		return nil, nil, nil, err
	}
	h.Prefix = imagePrefix(h)
	dn, err := randomBytes(NonceLen)
	if err != nil {
		Zero(kek)
		return nil, nil, nil, err
	}
	copy(h.DEKNonce[:], dn)
	g, err := newGCM(kek)
	if err != nil {
		Zero(kek)
		return nil, nil, nil, err
	}
	wrapped := g.Seal(nil, h.DEKNonce[:], ndek, aad(h.Prefix, "/dek/v1"))
	if newBody == nil {
		newBody, err = OpenBody(dek, ParsePrefix(image), h.BodyNonce, image[headerLen:])
		if err != nil {
			Zero(kek)
			Zero(ndek)
			return nil, nil, nil, err
		}
	}
	bn, ct, err := sealBody(ndek, h.Prefix, newBody)
	if err != nil {
		Zero(kek)
		Zero(ndek)
		return nil, nil, nil, err
	}
	out, err := serializeHeader(h, wrapped, bn)
	if err != nil {
		Zero(kek)
		Zero(ndek)
		return nil, nil, nil, err
	}
	return append(out, ct...), ndek, kek, nil
}

func ParsePrefix(image []byte) []byte {
	if len(image) < headerPrefix {
		return nil
	}
	return image[:headerPrefix]
}

func passwordMatches(kek []byte, h *Header, dek []byte) bool {
	g, err := newGCM(kek)
	if err != nil {
		return false
	}
	plain, err := g.Open(nil, h.DEKNonce[:], h.WrappedDEK[:], aad(h.Prefix, "/dek/v1"))
	if err != nil {
		return false
	}
	ok := subtle.ConstantTimeCompare(plain, dek) == 1
	Zero(plain)
	return ok
}

const proofContext = "agent-pwd/confirm/v1"

// Proof binds a confirmation id and nonce to the KEK derived from the master
// password. The password itself never crosses the IPC boundary.
func Proof(kek []byte, id, nonce string) string {
	mac := hmac.New(sha256.New, kek)
	mac.Write([]byte(proofContext))
	mac.Write([]byte{0})
	mac.Write([]byte(id))
	mac.Write([]byte{0})
	mac.Write([]byte(nonce))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyProof compares a proof in constant time.
func VerifyProof(kek []byte, id, nonce, proof string) bool {
	want := Proof(kek, id, nonce)
	return subtle.ConstantTimeCompare([]byte(want), []byte(proof)) == 1
}
