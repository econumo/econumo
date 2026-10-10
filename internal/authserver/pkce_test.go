package authserver

import "testing"

func TestVerifyPKCE(t *testing.T) {
	if !VerifyPKCE("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk", "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM") {
		t.Fatal("rfc vector")
	}
	if VerifyPKCE("wrong", "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM") {
		t.Fatal("wrong verifier accepted")
	}
	if VerifyPKCE("", "") {
		t.Fatal("empty accepted")
	}
}

func TestSecretHash(t *testing.T) {
	raw, hash, err := newSecret()
	if err != nil || len(raw) != 43 || hash != hashSecret(raw) || len(hash) != 64 {
		t.Fatalf("%q %q %v", raw, hash, err)
	}
	raw2, _, _ := newSecret()
	if raw == raw2 {
		t.Fatal("secrets must differ")
	}
}
