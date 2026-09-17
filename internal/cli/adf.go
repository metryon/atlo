package cli

import (
	"net/url"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	ext "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/util"
)

func adfNode(kind string, content []any) map[string]any {
	n := map[string]any{"type": kind}
	if content != nil {
		n["content"] = content
	}
	return n
}
func adfDocument(content []any) any { n := adfNode("doc", content); n["version"] = 1; return n }
func adfString(value string, marks []any) map[string]any {
	n := map[string]any{"type": "text", "text": value}
	if len(marks) > 0 {
		n["marks"] = marks
	}
	return n
}

// Plain text: blank-line runs separate paragraphs, individual newlines become
// hard breaks. Never add empty paragraphs for source formatting whitespace.
func adfText(value string) any {
	blocks := []any{}
	inline := []any{}
	flush := func() {
		if len(inline) > 0 {
			blocks = append(blocks, adfNode("paragraph", inline))
			inline = []any{}
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		if len(inline) > 0 {
			inline = append(inline, adfNode("hardBreak", nil))
		}
		inline = append(inline, adfString(line, nil))
	}
	flush()
	return adfDocument(blocks)
}

func markdownADF(value string) (any, error) {
	source := []byte(strings.ReplaceAll(value, "\r\n", "\n"))
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	root, err := parseMarkdown(md, source)
	if err != nil {
		return nil, err
	}
	c := adfConverter{source: source}
	blocks, err := c.blocks(root)
	if err != nil {
		return nil, err
	}
	if len(blocks) == 0 {
		return nil, invalid("Comment contains no renderable Markdown content.")
	}
	return adfDocument(blocks), nil
}

type adfConverter struct{ source []byte }

func (c adfConverter) blocks(parent ast.Node) ([]any, error) {
	out := []any{}
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		var result map[string]any
		switch node := n.(type) {
		case *ast.Paragraph, *ast.TextBlock, *ast.Heading:
			inline, err := c.inlines(n, nil)
			if err != nil {
				return nil, err
			}
			result = adfNode("paragraph", inline)
			if h, ok := n.(*ast.Heading); ok {
				result["type"] = "heading"
				result["attrs"] = map[string]any{"level": h.Level}
			}
		case *ast.List:
			children, err := c.blocks(n)
			if err != nil {
				return nil, err
			}
			result = adfNode("bulletList", children)
			if node.IsOrdered() {
				result["type"] = "orderedList"
				result["attrs"] = map[string]any{"order": node.Start}
			}
		case *ast.ListItem, *ast.Blockquote:
			children, err := c.blocks(n)
			if err != nil {
				return nil, err
			}
			kind := "blockquote"
			if _, ok := n.(*ast.ListItem); ok {
				kind = "listItem"
			}
			if len(children) == 0 {
				children = append(children, adfNode("paragraph", []any{}))
			}
			result = adfNode(kind, children)
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			var b strings.Builder
			for i := 0; i < n.Lines().Len(); i++ {
				segment := n.Lines().At(i)
				b.Write(segment.Value(c.source))
			}
			content := []any{}
			code := strings.TrimSuffix(b.String(), "\n")
			if code != "" {
				content = append(content, adfString(code, nil))
			}
			result = adfNode("codeBlock", content)
			if fenced, ok := n.(*ast.FencedCodeBlock); ok {
				if language := string(fenced.Language(c.source)); language != "" {
					result["attrs"] = map[string]any{"language": language}
				}
			}
		case *ast.ThematicBreak:
			result = adfNode("rule", nil)
		case *ext.Table:
			children, err := c.blocks(n)
			if err != nil {
				return nil, err
			}
			result = adfNode("table", children)
			result["attrs"] = map[string]any{"isNumberColumnEnabled": false, "layout": "default"}
		case *ext.TableHeader, *ext.TableRow:
			children, err := c.blocks(n)
			if err != nil {
				return nil, err
			}
			result = adfNode("tableRow", children)
		case *ext.TableCell:
			inline, err := c.inlines(n, nil)
			if err != nil {
				return nil, err
			}
			kind := "tableCell"
			if _, ok := n.Parent().(*ext.TableHeader); ok {
				kind = "tableHeader"
			}
			result = adfNode(kind, []any{adfNode("paragraph", inline)})
		case *ast.HTMLBlock:
			return nil, invalid("Raw HTML is not supported in Jira Markdown; use --format text for literal content or --adf for native formatting.")
		default:
			return nil, invalid("Unsupported Markdown block %s; use --format text or --adf.", n.Kind().String())
		}
		switch parent.(type) {
		case *ast.ListItem, *ast.Blockquote:
			switch result["type"] {
			case "paragraph", "bulletList", "orderedList", "codeBlock":
			default:
				return nil, invalid("Jira does not support %s inside %s; simplify the Markdown or use --format text.", result["type"], parent.Kind().String())
			}
		}
		out = append(out, result)
	}
	return out, nil
}
func decoded(value []byte) string {
	return string(util.ResolveEntityNames(util.ResolveNumericReferences(util.UnescapePunctuations(value))))
}
func appendMark(marks []any, kind string, attrs map[string]any) []any {
	out := append([]any{}, marks...)
	mark := map[string]any{"type": kind}
	if attrs != nil {
		mark["attrs"] = attrs
	}
	return append(out, mark)
}
func linkMark(marks []any, href, title string) ([]any, error) {
	u, err := url.Parse(href)
	if err != nil || u.Scheme == "" || (!strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") && !strings.EqualFold(u.Scheme, "mailto")) {
		return nil, invalid("Markdown links must use absolute http, https, or mailto URLs; use --format text for literal content.")
	}
	if (strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https")) && (u.Hostname() == "" || u.Opaque != "") {
		return nil, invalid("HTTP Markdown links must include a hostname.")
	}
	attrs := map[string]any{"href": href}
	if title != "" {
		attrs["title"] = title
	}
	return appendMark(marks, "link", attrs), nil
}
func (c adfConverter) inlines(parent ast.Node, marks []any) ([]any, error) {
	out := []any{}
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		switch node := n.(type) {
		case *ast.Text:
			value := string(node.Segment.Value(c.source))
			if !node.IsRaw() {
				value = decoded(node.Segment.Value(c.source))
			}
			if value != "" {
				out = append(out, adfString(value, marks))
			}
			if node.HardLineBreak() {
				out = append(out, adfNode("hardBreak", nil))
			} else if node.SoftLineBreak() {
				out = append(out, adfString(" ", marks))
			}
		case *ast.String:
			value := string(node.Value)
			if !node.IsRaw() {
				value = decoded(node.Value)
			}
			if value != "" {
				out = append(out, adfString(value, marks))
			}
		case *ast.CodeSpan:
			var b strings.Builder
			for child := n.FirstChild(); child != nil; child = child.NextSibling() {
				t, ok := child.(*ast.Text)
				if !ok {
					return nil, invalid("Unsupported code span content.")
				}
				b.Write(t.Segment.Value(c.source))
			}
			value := strings.ReplaceAll(b.String(), "\n", " ")
			if value != "" {
				codeMarks := []any{map[string]any{"type": "code"}}
				for _, mark := range marks {
					if asMap(mark)["type"] == "link" {
						codeMarks = append(codeMarks, mark)
					}
				}
				out = append(out, adfString(value, codeMarks))
			}
		case *ast.Emphasis:
			kind := "em"
			if node.Level == 2 {
				kind = "strong"
			}
			children, err := c.inlines(n, appendMark(marks, kind, nil))
			if err != nil {
				return nil, err
			}
			out = append(out, children...)
		case *ext.Strikethrough:
			children, err := c.inlines(n, appendMark(marks, "strike", nil))
			if err != nil {
				return nil, err
			}
			out = append(out, children...)
		case *ast.Link:
			next, err := linkMark(marks, decoded(node.Destination), decoded(node.Title))
			if err != nil {
				return nil, err
			}
			children, err := c.inlines(n, next)
			if err != nil {
				return nil, err
			}
			out = append(out, children...)
		case *ast.AutoLink:
			href := string(node.URL(c.source))
			if node.AutoLinkType == ast.AutoLinkEmail && !strings.HasPrefix(href, "mailto:") {
				href = "mailto:" + href
			}
			next, err := linkMark(marks, href, "")
			if err != nil {
				return nil, err
			}
			label := string(node.Label(c.source))
			if label != "" {
				out = append(out, adfString(label, next))
			}
		case *ext.TaskCheckBox:
			label := "[ ] "
			if node.IsChecked {
				label = "[x] "
			}
			out = append(out, adfString(label, marks))
		case *ast.Image:
			return nil, invalid("Markdown images require Jira attachment/media IDs; use a normal link or native --adf.")
		case *ast.RawHTML:
			return nil, invalid("Raw HTML is not supported in Jira Markdown; use --format text or --adf.")
		default:
			return nil, invalid("Unsupported Markdown inline %s; use --format text or --adf.", n.Kind().String())
		}
	}
	return out, nil
}
