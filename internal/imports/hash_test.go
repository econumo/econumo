package imports_test

import (
	"testing"

	"github.com/econumo/econumo/internal/imports"
)

func TestHashPayload(t *testing.T) {
	// sha256("abc"), a well-known vector.
	if got := imports.HashPayload([]byte("abc")); got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("HashPayload = %s", got)
	}
	if imports.HashPayload([]byte("abc")) == imports.HashPayload([]byte("abd")) {
		t.Fatal("distinct payloads must hash differently")
	}
}
