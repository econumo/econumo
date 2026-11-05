package model

import "testing"

func TestNormalizeEmail(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Owner@Example.TEST", "owner@example.test"},
		{"  padded@example.test\t", "padded@example.test"},
		{"JÖRG@example.test", "jörg@example.test"},
		{"jÖrg@example.test", "jörg@example.test"},
		{"ПЕТЯ@example.test", "петя@example.test"},
		{"admin@gmaİl.test", "admin@gmaİl.test"},
		{"Kate@example.test", "Kate@example.test"},
		{"ß@example.test", "ß@example.test"},
		{"", ""},
	}
	for _, c := range cases {
		got := NormalizeEmail(c.in)
		if got != c.want {
			t.Errorf("NormalizeEmail(%q) = %q, want %q", c.in, got, c.want)
		}
		if again := NormalizeEmail(got); again != got {
			t.Errorf("NormalizeEmail is not stable on %q: %q", got, again)
		}
	}
}
