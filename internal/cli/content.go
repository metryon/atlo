package cli

import (
	"bytes"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/metryon/atlo/internal/api"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

func textBody(a Args, stdin io.Reader) (string, error) {
	_, inline := a["body"]
	_, file := a["body-file"]
	if inline == file {
		return "", invalid("Supply exactly one of body or body-file.")
	}
	body := a.S("body")
	if len(body) > api.MaxBytes {
		return "", invalid("Input exceeds 16 MiB.")
	}
	if file {
		var b []byte
		var err error
		if a.S("body-file") == "-" {
			b, err = readBounded(stdin)
		} else {
			b, err = readFile(a.S("body-file"))
		}
		if err != nil {
			return "", err
		}
		body = string(b)
	}
	if !utf8.ValidString(body) {
		return "", invalid("Body must be valid UTF-8.")
	}
	if strings.TrimSpace(body) == "" {
		return "", invalid("Body must not be empty.")
	}
	return body, nil
}
func storageBody(a Args, stdin io.Reader) (string, error) {
	body, err := textBody(a, stdin)
	if err != nil {
		return "", err
	}
	if a.S("format") == "storage" {
		return body, nil
	}
	var out bytes.Buffer
	md := goldmark.New(goldmark.WithExtensions(extension.GFM), goldmark.WithRendererOptions(html.WithXHTML()))
	source := []byte(body)
	root, err := parseMarkdown(md, source)
	if err != nil {
		return "", err
	}
	if err := md.Renderer().Render(&out, source, root); err != nil {
		return "", invalid("Could not convert Markdown.")
	}
	return out.String(), nil
}
