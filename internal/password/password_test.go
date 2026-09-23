package password

import (
	"strings"
	"testing"
	"time"
)

func TestHashAndVerify(t *testing.T) {
	hash, err := Hash("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "correct horse" {
		t.Fatal("password was not hashed")
	}
	if !Verify(hash, "correct horse") {
		t.Fatal("expected valid password to verify")
	}
	if Verify(hash, "wrong") {
		t.Fatal("expected wrong password to fail")
	}
}

func TestHashRejectsShort(t *testing.T) {
	if _, err := Hash("abc"); err == nil {
		t.Fatal("expected error for short password")
	}
}

func TestCookieSigner(t *testing.T) {
	signer := NewCookieSigner("test-secret")
	token := signer.Sign("myslug", time.Now().Add(time.Hour))
	if !signer.Valid("myslug", token) {
		t.Fatal("expected token to be valid")
	}
	if signer.Valid("otherslug", token) {
		t.Fatal("token must not validate for another slug")
	}
	if signer.Valid("myslug", token+"tampered") {
		t.Fatal("tampered token must not validate")
	}
	if signer.Valid("myslug", "garbage") {
		t.Fatal("garbage token must not validate")
	}
	// Expired tokens are rejected server-side, not just by cookie Max-Age.
	expired := signer.Sign("myslug", time.Now().Add(-time.Minute))
	if signer.Valid("myslug", expired) {
		t.Fatal("expired token must not validate")
	}
	// Tokens embed the slug; slugs with dots still round-trip.
	token2 := signer.Sign("a.b", time.Now().Add(time.Hour))
	if !strings.Contains(token2, ".") || !signer.Valid("a.b", token2) {
		t.Fatal("expected slug with dot to round-trip")
	}
}
