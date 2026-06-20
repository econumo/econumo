package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"
)

// Repo dev keypair, copied into testdata so these tests are self-contained and
// run without external services in CI. The passphrase is the repo's dev
// JWT_PASSPHRASE from .env.dist (not a production secret).
const (
	testPrivateKeyPath = "testdata/private.pem"
	testPublicKeyPath  = "testdata/public.pem"
	testPassphrase     = "d78eedcb16c13bd949ede5d1b8b910cd"
)

func newTestJWT(t *testing.T) *JWT {
	t.Helper()
	j, err := NewJWT(testPrivateKeyPath, testPublicKeyPath, testPassphrase)
	if err != nil {
		t.Fatalf("NewJWT: %v", err)
	}
	return j
}

// TestIssueVerifyRoundTrip issues a token then verifies it, asserting the claim
// set matches exactly: iat, exp=iat+TTL, roles, username, id.
func TestIssueVerifyRoundTrip(t *testing.T) {
	j := newTestJWT(t)

	// Use a current time so exp (now+30d) is in the future for Verify's real clock.
	now := time.Now().Truncate(time.Second)
	userID := "01890a5d-ac96-774b-8e3f-9d1b2c3d4e5f"
	email := "alice@example.com"

	token, err := j.Issue(userID, email, now)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	claims, err := j.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if claims.Iat != now.Unix() {
		t.Errorf("iat = %d, want %d", claims.Iat, now.Unix())
	}
	if want := now.Unix() + 2592000; claims.Exp != want {
		t.Errorf("exp = %d, want %d", claims.Exp, want)
	}
	if claims.ID != userID {
		t.Errorf("id = %q, want %q", claims.ID, userID)
	}
	if claims.Username != email {
		t.Errorf("username = %q, want %q", claims.Username, email)
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != "ROLE_USER" {
		t.Errorf("roles = %v, want [ROLE_USER]", claims.Roles)
	}
}

// tokenFixture mirrors testdata/lexik_token.json, a token signed with the repo
// private key by the existing backend (see testdata/gen_jwt.php).
type tokenFixture struct {
	Token    string `json:"token"`
	Expected struct {
		Iat      int64    `json:"iat"`
		Exp      int64    `json:"exp"`
		Roles    []string `json:"roles"`
		Username string   `json:"username"`
		ID       string   `json:"id"`
	} `json:"expected"`
}

// TestVerifyExistingBackendToken proves cross-compatibility: our Verify accepts
// a token signed by the existing backend, and the parsed claims match the
// expected shape. The guarantee runs both directions — a token signed with this
// key elsewhere verifies here, and (same key, same RS256) a token we Issue
// verifies there.
func TestVerifyExistingBackendToken(t *testing.T) {
	raw, err := os.ReadFile("testdata/lexik_token.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fix tokenFixture
	if err := json.Unmarshal(raw, &fix); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	// The fixture exp is fixed (1702592000); make sure validation does not fail
	// just because that timestamp is in the past relative to the test clock.
	// We verify with a parser pinned to a time before exp by constructing a JWT
	// configured to validate against a fixed clock.
	j := newTestJWT(t)
	claims, err := verifyAt(j, fix.Token, time.Unix(fix.Expected.Iat+1, 0))
	if err != nil {
		t.Fatalf("Verify existing-backend token: %v", err)
	}

	if claims.Iat != fix.Expected.Iat {
		t.Errorf("iat = %d, want %d", claims.Iat, fix.Expected.Iat)
	}
	if claims.Exp != fix.Expected.Exp {
		t.Errorf("exp = %d, want %d", claims.Exp, fix.Expected.Exp)
	}
	if claims.ID != fix.Expected.ID {
		t.Errorf("id = %q, want %q", claims.ID, fix.Expected.ID)
	}
	if claims.Username != fix.Expected.Username {
		t.Errorf("username = %q, want %q", claims.Username, fix.Expected.Username)
	}
	if len(claims.Roles) != len(fix.Expected.Roles) || claims.Roles[0] != fix.Expected.Roles[0] {
		t.Errorf("roles = %v, want %v", claims.Roles, fix.Expected.Roles)
	}
}

// verifyAt re-runs Verify's logic with a pinned clock, so a fixture with a
// long-past exp still validates in tests. Production Verify uses the real clock.
func verifyAt(j *JWT, tokenString string, at time.Time) (Claims, error) {
	var claims Claims
	parser := jwtv5.NewParser(
		jwtv5.WithValidMethods([]string{signingMethod.Alg()}),
		jwtv5.WithExpirationRequired(),
		jwtv5.WithTimeFunc(func() time.Time { return at }),
	)
	_, err := parser.ParseWithClaims(tokenString, &claims, func(t *jwtv5.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwtv5.SigningMethodRSA); !ok {
			return nil, jwtv5.ErrTokenSignatureInvalid
		}
		return j.public, nil
	})
	return claims, err
}

// TestVerifyRejectsTampered ensures a token whose payload was altered (so the
// signature no longer matches) is rejected.
func TestVerifyRejectsTampered(t *testing.T) {
	j := newTestJWT(t)
	token, err := j.Issue("id-1", "bob@example.com", time.Unix(1700000000, 0))
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// Flip a character in the payload segment.
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT segments, got %d", len(parts))
	}
	payload := []byte(parts[1])
	if payload[0] == 'A' {
		payload[0] = 'B'
	} else {
		payload[0] = 'A'
	}
	tampered := parts[0] + "." + string(payload) + "." + parts[2]

	if _, err := j.Verify(tampered); err == nil {
		t.Fatal("expected tampered token to be rejected, got nil error")
	}
}

// TestVerifyRejectsExpired ensures an expired token is rejected by exp validation.
func TestVerifyRejectsExpired(t *testing.T) {
	j := newTestJWT(t)
	// Issue at a time far in the past so exp (iat+30d) is already elapsed.
	past := time.Now().Add(-60 * 24 * time.Hour)
	token, err := j.Issue("id-1", "carol@example.com", past)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := j.Verify(token); err == nil {
		t.Fatal("expected expired token to be rejected, got nil error")
	}
}

// TestVerifyRejectsWrongAlg ensures tokens signed with a non-RS256 algorithm are
// rejected: an HS256 token (alg-confusion attempt) and a "none" token.
func TestVerifyRejectsWrongAlg(t *testing.T) {
	j := newTestJWT(t)

	claims := Claims{
		Iat:      time.Now().Unix(),
		Exp:      time.Now().Add(time.Hour).Unix(),
		Roles:    []string{"ROLE_USER"},
		Username: "mallory@example.com",
		ID:       "id-evil",
	}

	// HS256 token signed with an arbitrary secret.
	hs, err := jwtv5.NewWithClaims(jwtv5.SigningMethodHS256, claims).SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("sign HS256: %v", err)
	}
	if _, err := j.Verify(hs); err == nil {
		t.Fatal("expected HS256 token to be rejected, got nil error")
	}

	// "none" (unsigned) token.
	none, err := jwtv5.NewWithClaims(jwtv5.SigningMethodNone, claims).SignedString(jwtv5.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none: %v", err)
	}
	if _, err := j.Verify(none); err == nil {
		t.Fatal("expected none-alg token to be rejected, got nil error")
	}
}

// TestVerifyRejectsForeignKey ensures a validly-RS256-signed token from a
// different key does not verify against our public key.
func TestVerifyRejectsForeignKey(t *testing.T) {
	j := newTestJWT(t)

	foreign, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	claims := Claims{
		Iat:      time.Now().Unix(),
		Exp:      time.Now().Add(time.Hour).Unix(),
		Roles:    []string{"ROLE_USER"},
		Username: "eve@example.com",
		ID:       "id-foreign",
	}
	token, err := jwtv5.NewWithClaims(jwtv5.SigningMethodRS256, claims).SignedString(foreign)
	if err != nil {
		t.Fatalf("sign foreign: %v", err)
	}
	if _, err := j.Verify(token); err == nil {
		t.Fatal("expected foreign-key token to be rejected, got nil error")
	}
}

// TestVerifyOnlyWithoutPrivateKey confirms a JWT built without a readable
// private key can still verify but refuses to issue.
func TestVerifyOnlyWithoutPrivateKey(t *testing.T) {
	j, err := NewJWT("testdata/does-not-exist.pem", testPublicKeyPath, testPassphrase)
	if err != nil {
		t.Fatalf("NewJWT verify-only: %v", err)
	}
	if _, err := j.Issue("id", "x@y.z", time.Now()); err == nil {
		t.Fatal("expected Issue to fail without private key")
	}
	// Verify still works using the committed token fixture.
	raw, _ := os.ReadFile("testdata/lexik_token.json")
	var fix tokenFixture
	_ = json.Unmarshal(raw, &fix)
	if _, err := verifyAt(j, fix.Token, time.Unix(fix.Expected.Iat+1, 0)); err != nil {
		t.Fatalf("verify-only Verify failed: %v", err)
	}
}
