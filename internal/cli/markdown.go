package cli

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Inspect iteratively before either recursive renderer expands the parsed tree.
func parseMarkdown(md goldmark.Markdown, source []byte) (ast.Node, error) {
	root := md.Parser().Parse(text.NewReader(source))
	depth, count := 0, 0
	for node := root; node != nil; {
		count++
		if depth > 64 || count > 100000 {
			return nil, invalid("Markdown exceeds 64 nesting levels or 100000 nodes; simplify the document or use native content.")
		}
		if child := node.FirstChild(); child != nil {
			node = child
			depth++
			continue
		}
		for node != root && node.NextSibling() == nil {
			node = node.Parent()
			depth--
		}
		if node == root {
			break
		}
		node = node.NextSibling()
	}
	return root, nil
}
