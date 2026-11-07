package auth

import (
	"strings"
	"testing"
)

func TestLooksEncrypted(t *testing.T) {
	for _, plain := range []string{"Alice@Example.test", "x", "JÖRG@example.test", strings.Repeat("Long@Address.Example.Test", 8)} {
		out, err := NewEncodeService("0123456789abcdef").Encode(plain)
		if err != nil {
			t.Fatal(err)
		}
		if !LooksEncrypted(out) {
			t.Errorf("ciphertext of %q not recognised: %q", plain, out)
		}
	}
	for _, plain := range []string{"", "Admin", "Alice@Example.test", "QUJDREVGR0hJSktMTU5PUA==", strings.Repeat("A", 86) + "@x"} {
		if LooksEncrypted(plain) {
			t.Errorf("%q taken for ciphertext", plain)
		}
	}
}
