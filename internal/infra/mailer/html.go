package mailer

import (
	"bytes"
	_ "embed"
	"html/template"
)

// LogoContentID is the Content-ID the layout's <img src="cid:..."> points at.
const LogoContentID = "econumo-logo"

// Logo is the PNG attached inline to every HTML email.
//
//go:embed templates/logo.png
var Logo []byte

//go:embed templates/layout.html
var layoutSource string

var layout = template.Must(template.New("layout").Parse(layoutSource))

// RenderHTML renders c into the shared layout; html/template escapes every field.
func RenderHTML(c Content) (string, error) {
	var buf bytes.Buffer
	if err := layout.Execute(&buf, c); err != nil {
		return "", err
	}
	return buf.String(), nil
}
