package backend

import (
	"context"
	"database/sql"
	"testing"
)

type fakeBackend struct{ name string }

func (f *fakeBackend) Name() string { return f.name }
func (f *fakeBackend) Open(ctx context.Context, dsn string) (*sql.DB, error) {
	return nil, nil
}
func (f *fakeBackend) Migrations() []Migration { return nil }

func TestRegisterGetRegistered(t *testing.T) {
	name := "backend-test-fake"
	if _, ok := Get(name); ok {
		t.Fatalf("Get(%q) found a backend before Register", name)
	}

	b := &fakeBackend{name: name}
	Register(name, b)

	got, ok := Get(name)
	if !ok {
		t.Fatalf("Get(%q) not found after Register", name)
	}
	if got != Backend(b) {
		t.Errorf("Get(%q) returned a different backend instance", name)
	}

	found := false
	for _, n := range Registered() {
		if n == name {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Registered() = %v, want it to contain %q", Registered(), name)
	}
}

func TestGet_UnknownEngine(t *testing.T) {
	if _, ok := Get("no-such-engine-xyz"); ok {
		t.Error("Get on an unregistered engine name returned ok=true, want false")
	}
}

func TestRegister_DuplicatePanics(t *testing.T) {
	name := "backend-test-dup"
	Register(name, &fakeBackend{name: name})

	defer func() {
		if r := recover(); r == nil {
			t.Error("Register called twice for the same name: expected a panic, got none")
		}
	}()
	Register(name, &fakeBackend{name: name})
}

func TestRegister_NilPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Register called with a nil Backend: expected a panic, got none")
		}
	}()
	Register("backend-test-nil", nil)
}
