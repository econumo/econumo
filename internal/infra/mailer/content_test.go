package mailer

import (
	"strings"
	"testing"
)

func TestRenderText_ResetEnglish(t *testing.T) {
	got := RenderText(Compose("en", "reset", nil, "Anna", "482913", "https://app.econumo.test"))
	want := "Hi Anna,\n\nEnter this code in Econumo to choose a new password.\n\nYour confirmation code is: 482913.\n\nIf you didn't request this code, you can ignore this email. Your password stays the same.\n\n--\nEconumo — Manage money. Together.\nhttps://app.econumo.test\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderText_NoAppURL(t *testing.T) {
	got := RenderText(Compose("en", "reset", nil, "Anna", "482913", ""))
	if !strings.HasSuffix(got, "Together.\n") || strings.Contains(got, "http") {
		t.Fatalf("unexpected footer: %q", got)
	}
}

func TestRenderText_NoticeHasNoCodeLine(t *testing.T) {
	got := RenderText(Compose("en", "identity_linked", map[string]any{"provider": "Google"}, "Anna", "", "https://app.econumo.test"))
	if strings.Contains(got, "confirmation code") || !strings.Contains(got, "Your Google account") {
		t.Fatalf("unexpected text: %q", got)
	}
}

func TestCompose_EmptyNameUsesAnonymousGreeting(t *testing.T) {
	if g := Compose("en", "reset", nil, "  ", "1", "").Greeting; g != "Hi," {
		t.Fatalf("greeting = %q", g)
	}
}

func TestCompose_RussianCode(t *testing.T) {
	if got := RenderText(Compose("ru", "reset", nil, "Анна", "482913", "")); !strings.Contains(got, "482913") {
		t.Fatalf("code missing: %q", got)
	}
}

func TestParagraphs(t *testing.T) {
	c := Content{Body: " one \n\n\n\ntwo\n\n"}
	if p := c.Paragraphs(); len(p) != 2 || p[0] != "one" || p[1] != "two" {
		t.Fatalf("paragraphs = %q", p)
	}
}
