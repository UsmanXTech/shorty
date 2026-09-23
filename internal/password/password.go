package password

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidPassword = errors.New("invalid password")

// hashCost is the bcrypt cost used by Hash. Tests lower it to bcrypt.MinCost
// to keep the suite fast; production always uses DefaultCost.
var hashCost = bcrypt.DefaultCost

// SetTestCost lowers the bcrypt cost for tests. It must only be used in
// tests; production code always hashes with bcrypt.DefaultCost.
func SetTestCost(cost int) { hashCost = cost }

// Hash returns a bcrypt hash of the given password.
func Hash(password string) (string, error) {
	if len(password) < 4 {
		return "", fmt.Errorf("password must be at least 4 characters")
	}
	sum, err := bcrypt.GenerateFromPassword([]byte(password), hashCost)
	if err != nil {
		return "", err
	}
	return string(sum), nil
}

// Verify reports whether password matches the bcrypt hash.
func Verify(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// NewCookieSigner builds an HMAC signer for auth cookies from a secret.
// If secret is empty, a random one is generated (cookies then expire on restart).
func NewCookieSigner(secret string) *CookieSigner {
	key := []byte(secret)
	if len(key) == 0 {
		key = make([]byte, 32)
		// A failed CSPRNG read must not silently yield an all-zero key.
		if _, err := rand.Read(key); err != nil {
			panic("password: crypto/rand failed: " + err.Error())
		}
	}
	return &CookieSigner{key: key}
}

type CookieSigner struct {
	key []byte
}

// Sign returns a token proving the holder unlocked slug until expiresAt.
// The expiry is embedded in the signed payload so it is enforced
// server-side: a captured token cannot be replayed past its lifetime by
// setting the Cookie header manually.
func (s *CookieSigner) Sign(slug string, expiresAt time.Time) string {
	body := base64.RawURLEncoding.EncodeToString([]byte(slug)) + "." +
		base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(expiresAt.UTC().Unix(), 10)))
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte("shorty-unlock:" + body))
	return body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Valid reports whether token is a valid, unexpired unlock token for slug.
func (s *CookieSigner) Valid(slug, token string) bool {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return false
	}
	rawSlug, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || string(rawSlug) != slug {
		return false
	}
	rawExp, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	expUnix, err := strconv.ParseInt(string(rawExp), 10, 64)
	if err != nil || time.Now().UTC().Unix() > expUnix {
		return false
	}
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte("shorty-unlock:" + parts[0] + "." + parts[1]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(parts[2]), []byte(expected))
}
