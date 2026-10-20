package secrets

import (
	"bytes"
	"testing"
)

func TestRoundTripAndTamper(t *testing.T) {
	b, err := New("a-sufficiently-long-test-key")
	if err != nil {
		t.Fatal(err)
	}
	ct, err := b.Seal([]byte("token-123"), []byte("int-1"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ct, []byte("token-123")) {
		t.Fatal("plaintext visible in ciphertext")
	}
	pt, err := b.Open(ct, []byte("int-1"))
	if err != nil || string(pt) != "token-123" {
		t.Fatalf("open: %v %q", err, pt)
	}
	if _, err := b.Open(ct, []byte("int-2")); err == nil {
		t.Fatal("wrong aad must fail")
	}
	ct[len(ct)-1] ^= 1
	if _, err := b.Open(ct, []byte("int-1")); err == nil {
		t.Fatal("tampered ciphertext must fail")
	}
	other, _ := New("another-long-test-key-xx")
	ct2, _ := b.Seal([]byte("x"), nil)
	if _, err := other.Open(ct2, nil); err == nil {
		t.Fatal("wrong key must fail")
	}
	if _, err := New("short"); err == nil {
		t.Fatal("short key must be rejected")
	}
}
