package auth

import "testing"

func TestPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(h, "correct horse battery staple") {
		t.Fatal("expected verify to succeed")
	}
	if VerifyPassword(h, "wrong") {
		t.Fatal("expected verify to fail")
	}
	if VerifyPassword("garbage", "x") {
		t.Fatal("garbage hash must fail")
	}
}

func TestTokens(t *testing.T) {
	tok, hash, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) < 40 || len(hash) != 32 {
		t.Fatalf("unexpected token sizes %d %d", len(tok), len(hash))
	}
	if string(HashToken(tok)) != string(hash) {
		t.Fatal("hash mismatch")
	}
}
