package crypto

import (
	"bytes"
	"errors"
	"testing"
)

func testParams() KDFParams { return KDFParams{Time: 1, MemoryKiB: 1024, Parallelism: 1} }

func TestRoundTripAndTamper(t *testing.T) {
	plain := []byte(`{"message":"hello"}`)
	image, dek, kek, err := CreateWithParams("correct horse", plain, testParams())
	if err != nil {
		t.Fatal(err)
	}
	_, _, got, _, err := Open("correct horse", image)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("roundtrip: %v %q", err, got)
	}
	if len(dek) != KeyLen || len(kek) != KeyLen {
		t.Fatal("bad key lengths")
	}
	if _, _, _, _, err := Open("wrong horse", image); !errors.Is(err, ErrBadPassword) {
		t.Fatalf("wrong password: %v", err)
	}
	tampered := append([]byte(nil), image...)
	tampered[len(tampered)-1] ^= 1
	if _, _, _, _, err := Open("correct horse", tampered); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("ciphertext tamper: %v", err)
	}
	tampered = append([]byte(nil), image...)
	tampered[0] ^= 1
	if _, _, _, _, err := Open("correct horse", tampered); !errors.Is(err, ErrBadMagic) {
		t.Fatalf("header tamper: %v", err)
	}
	tampered = append([]byte(nil), image...)
	tampered[10] ^= 1 // vault id is inside the AAD prefix
	if _, _, _, _, err := Open("correct horse", tampered); !errors.Is(err, ErrCorrupt) && !errors.Is(err, ErrBadPassword) {
		t.Fatalf("AAD tamper: %v", err)
	}
}

func TestRewrap(t *testing.T) {
	plain := []byte(`{"n":42}`)
	image, dek, kek, err := CreateWithParams("old", plain, testParams())
	if err != nil {
		t.Fatal(err)
	}
	for _, newSalt := range []bool{false, true} {
		next, _, newKEK, err := Rewrap(image, kek, dek, "new", nil, newSalt)
		if err != nil {
			t.Fatalf("rewrap newSalt=%v: %v", newSalt, err)
		}
		_, _, body, h, err := Open("new", next)
		if err != nil || !bytes.Equal(body, plain) {
			t.Fatalf("open after rewrap newSalt=%v: %v %q", newSalt, err, body)
		}
		if h.Version != Version {
			t.Fatal("version changed")
		}
		if _, _, _, _, err := Open("old", next); !errors.Is(err, ErrBadPassword) {
			t.Fatalf("old password still works (newSalt=%v)", newSalt)
		}
		cryptoZero(newKEK)
	}
}

func cryptoZero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func TestProofBinding(t *testing.T) {
	kek := bytes.Repeat([]byte{7}, KeyLen)
	p := Proof(kek, "c_1", "nonce")
	if !VerifyProof(kek, "c_1", "nonce", p) {
		t.Fatal("proof did not verify")
	}
	if VerifyProof(kek, "c_2", "nonce", p) || VerifyProof(kek, "c_1", "other", p) {
		t.Fatal("proof accepted with wrong binding")
	}
}

func TestDefaultKDF(t *testing.T) {
	p := DefaultKDF()
	if p.Time != 3 || p.MemoryKiB != 64*1024 || p.Parallelism != 4 {
		t.Fatalf("KDF defaults changed: %+v", p)
	}
}
