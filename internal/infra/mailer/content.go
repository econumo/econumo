package mailer

import (
	"strings"

	"github.com/econumo/econumo/internal/infra/i18n"
)

// Content is one email broken into the parts every rendering shares. The
// greeting, code line, signature and app URL belong to the template, so a
// catalogue entry carries only heading, body and note.
type Content struct {
	Lang, Heading, Greeting, Body, Code, CodeLabel, CodeText, Note, Tagline, OpenApp, AppURL string
}

// Compose reads emails.<key>.{heading,body,note} with params and the shared
// emails.common parts. Code parts are filled only when code is non-empty, so
// notices carry no code line.
func Compose(lang, key string, params map[string]any, name, code, appURL string) Content {
	t := func(k string, p map[string]any) string { return i18n.T(lang, "emails."+k, p) }
	c := Content{
		Lang:    lang,
		Heading: t(key+".heading", params),
		Body:    t(key+".body", params),
		Note:    t(key+".note", params),
		Tagline: t("common.tagline", nil),
		OpenApp: t("common.open_app", nil),
		AppURL:  appURL,
	}
	if strings.TrimSpace(name) == "" {
		c.Greeting = t("common.greeting_anonymous", nil)
	} else {
		c.Greeting = t("common.greeting", map[string]any{"name": name})
	}
	if code != "" {
		c.Code = code
		c.CodeLabel = t("common.code_label", nil)
		c.CodeText = t("common.code_text", map[string]any{"code": code})
	}
	return c
}

// Paragraphs splits Body on blank lines.
func (c Content) Paragraphs() []string {
	var out []string
	for _, p := range strings.Split(c.Body, "\n\n") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// RenderText renders the plain-text alternative; empty blocks are omitted
// together with their blank line.
func RenderText(c Content) string {
	var blocks []string
	for _, b := range []string{c.Greeting, c.Body, c.CodeText, c.Note} {
		if b != "" {
			blocks = append(blocks, b)
		}
	}
	sig := "--\nEconumo — " + c.Tagline + "\n"
	if c.AppURL != "" {
		sig += c.AppURL + "\n"
	}
	return strings.Join(blocks, "\n\n") + "\n\n" + sig
}
