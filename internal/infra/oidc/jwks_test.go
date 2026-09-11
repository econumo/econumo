package oidc

import "testing"

func TestJWKPublicKey(t *testing.T) {
	rsaKey := jwk{Kty: "RSA", N: "AQAB", E: "AQAB"}
	if _, err := rsaKey.publicKey(); err != nil {
		t.Fatalf("valid RSA jwk: %v", err)
	}
	if _, err := (jwk{Kty: "RSA", N: "not-base64!", E: "AQAB"}).publicKey(); err == nil {
		t.Fatal("bad N must fail")
	}
	if _, err := (jwk{Kty: "RSA", N: "AQAB", E: "not-base64!"}).publicKey(); err == nil {
		t.Fatal("bad E must fail")
	}

	ecKey := jwk{Kty: "EC", Crv: "P-256", X: "AQAB", Y: "AQAB"}
	if _, err := ecKey.publicKey(); err != nil {
		t.Fatalf("valid EC jwk: %v", err)
	}
	if _, err := (jwk{Kty: "EC", Crv: "P-384", X: "AQAB", Y: "AQAB"}).publicKey(); err == nil {
		t.Fatal("unsupported curve must fail")
	}
	if _, err := (jwk{Kty: "EC", Crv: "P-256", X: "not-base64!", Y: "AQAB"}).publicKey(); err == nil {
		t.Fatal("bad X must fail")
	}
	if _, err := (jwk{Kty: "EC", Crv: "P-256", X: "AQAB", Y: "not-base64!"}).publicKey(); err == nil {
		t.Fatal("bad Y must fail")
	}

	if _, err := (jwk{Kty: "oct"}).publicKey(); err == nil {
		t.Fatal("unsupported kty must fail")
	}
}
