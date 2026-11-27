package protocol

import (
	"bytes"
	"strings"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	payload := []byte(`{"op":"status"}`)
	if err := WriteFrame(&buf, payload); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFrame(&buf)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("roundtrip: %q %v", got, err)
	}
}

func TestFrameRejectsOversize(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, make([]byte, MaxFrameSize+1)); err != ErrFrameTooLarge {
		t.Fatalf("write: %v", err)
	}
	huge := []byte{0x00, 0x40, 0x00, 0x01}
	if _, err := ReadFrame(bytes.NewReader(huge)); err != ErrFrameTooLarge {
		t.Fatalf("read: %v", err)
	}
}

func TestExitCodes(t *testing.T) {
	cases := map[string]int{
		CodeOK: 0, CodePending: 10, CodeLocked: 20, CodeNoDaemon: 20,
		CodeDenied: 30, CodeNotFound: 40, CodeInternal: 1,
	}
	for code, want := range cases {
		if got := ExitCode(code); got != want {
			t.Errorf("ExitCode(%s)=%d want %d", code, got, want)
		}
	}
	if strings.Contains(CodePending, " ") {
		t.Fatal("code has whitespace")
	}
}
