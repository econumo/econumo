package mailer

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const testAppURL = "https://app.econumo.test"

var goldenEmails = []struct {
	key    string
	params map[string]any
	code   string
}{
	{"reset", nil, "482913"},
	{"verify", nil, "482913"},
	{"change_email", map[string]any{"email": "new@example.test"}, "482913"},
	{"change_email_notice", map[string]any{"email": "new@example.test"}, ""},
	{"identity_linked", map[string]any{"provider": "Google"}, ""},
	{"identity_unlinked", map[string]any{"provider": "Google"}, ""},
}

func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s (run with UPDATE_GOLDEN=1): %v", path, err)
	}
	if string(want) != got {
		t.Fatalf("%s differs from golden (run with UPDATE_GOLDEN=1 and inspect the diff)", name)
	}
}

func TestRenderHTML_Golden(t *testing.T) {
	for _, e := range goldenEmails {
		t.Run(e.key, func(t *testing.T) {
			c := Compose("en", e.key, e.params, "Anna", e.code, testAppURL)
			html, err := RenderHTML(c)
			if err != nil {
				t.Fatal(err)
			}
			checkGolden(t, e.key+".html", html)
			checkGolden(t, e.key+".txt", RenderText(c))
		})
	}
}

func TestRenderHTML_EscapesUserStrings(t *testing.T) {
	name := `<b>Ann & "Bo"</b>`
	c := Compose("en", "reset", nil, name, "482913", testAppURL)
	html, err := RenderHTML(c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `&lt;b&gt;Ann &amp; &#34;Bo&#34;&lt;/b&gt;`) || strings.Contains(html, "<b>Ann") {
		t.Fatalf("name not escaped: %s", html)
	}
	if !strings.Contains(RenderText(c), name) {
		t.Fatal("text must carry the literal name")
	}
}

func TestRenderHTML_NoAppURLNoButton(t *testing.T) {
	html, err := RenderHTML(Compose("en", "reset", nil, "Anna", "482913", ""))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<a href") || strings.Contains(html, "Open Econumo") {
		t.Fatal("button rendered without an app URL")
	}
}

func TestRenderHTML_BodyWraps(t *testing.T) {
	html, err := RenderHTML(Compose("en", "reset", nil, "Anna", "482913", testAppURL))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "word-break:break-word") {
		t.Fatal("body paragraphs must wrap long words")
	}
}

func TestRenderHTML_NoRemoteResources(t *testing.T) {
	html, err := RenderHTML(Compose("en", "reset", nil, "Anna", "482913", testAppURL))
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`src="https?://`).MatchString(html) {
		t.Fatal("remote src found")
	}
	if !strings.Contains(html, `src="cid:`+LogoContentID+`"`) {
		t.Fatal("logo cid reference missing")
	}
}

func TestLogoIsPNG(t *testing.T) {
	if !strings.HasPrefix(string(Logo), "\x89PNG") {
		t.Fatal("Logo is not a PNG")
	}
}
