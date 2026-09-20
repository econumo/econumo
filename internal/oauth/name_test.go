package oauth

import "testing"

func TestDeriveName(t *testing.T) {
	cases := []struct{ name, email, want string }{
		{"Alice Example", "a@x.test", "Alice Example"},
		{"  Al  ", "alice.smith@x.test", "alice.smith"},
		{"", "ab@x.test", "User"},
		{"Bartholomew Montgomery-Fitzgerald III", "b@x.test", "Bartholomew Montgome"},
		{"Zoë", "z@x.test", "Zoë"},
	}
	for _, c := range cases {
		if got := deriveName(c.name, c.email); got != c.want {
			t.Errorf("deriveName(%q,%q) = %q, want %q", c.name, c.email, got, c.want)
		}
	}
}

func TestAppleName(t *testing.T) {
	if got := appleName(`{"name":{"firstName":"Ada","lastName":"Lovelace"}}`); got != "Ada Lovelace" {
		t.Errorf("appleName(valid) = %q", got)
	}
	if got := appleName("not json"); got != "" {
		t.Errorf("appleName(invalid) = %q, want empty", got)
	}
}
