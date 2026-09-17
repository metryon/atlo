package cli

import "strings"

// A single registry drives parsing, discovery and validation so the executable
// contract cannot drift away from its agent-readable schema.
type Property struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Default     any      `json:"default,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Minimum     int      `json:"minimum,omitempty"`
	Maximum     int      `json:"maximum,omitempty"`
}
type Command struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Mutation    bool                `json:"mutation"`
	Positionals []string            `json:"positionals,omitempty"`
	Properties  map[string]Property `json:"properties"`
	Required    []string            `json:"required,omitempty"`
}

func str(d string) Property { return Property{Type: "string", Description: d} }
func obj(d string) Property { return Property{Type: "object", Description: d} }
func integer(d string, def int) Property {
	p := Property{Type: "integer", Description: d, Minimum: 1}
	if def != 0 {
		p.Default = def
	}
	return p
}
func choice(d string, def string, values ...string) Property {
	return Property{Type: "string", Description: d, Default: def, Enum: values}
}
func props(pairs ...any) map[string]Property {
	p := map[string]Property{}
	for i := 0; i < len(pairs); i += 2 {
		p[pairs[i].(string)] = pairs[i+1].(Property)
	}
	return p
}
func paginated(p map[string]Property) map[string]Property {
	p["limit"] = integer("Maximum records in this response (1–100).", 10)
	limit := p["limit"]
	limit.Maximum = 100
	p["limit"] = limit
	p["cursor"] = str("Opaque continuation token returned in meta.next_cursor; reuse the same filters.")
	return p
}

var commands = []Command{
	{Name: "jira attachment upload", Description: "Upload one local file to an issue; streams bytes, never retries. Dry-run shows file metadata only.", Mutation: true, Positionals: []string{"key"}, Required: []string{"key", "file"}, Properties: uploadProperties("jira")},
	{Name: "confluence attachment upload", Description: "Create one page attachment without replacing an existing filename; does not embed it in the page. Dry-run shows metadata only.", Mutation: true, Positionals: []string{"id"}, Required: []string{"id", "file"}, Properties: uploadProperties("confluence")},
	{Name: "confluence attachment list", Description: "List a bounded page of attachment metadata for a page.", Positionals: []string{"id"}, Required: []string{"id"}, Properties: paginated(props("id", str("Numeric page ID.")))},
	{Name: "auth check", Description: "Check API-token authentication for one product.", Properties: props("product", choice("Product to check.", "jira", "jira", "confluence"))},
	{Name: "jira issue get", Description: "Read a compact issue; request extra fields explicitly.", Positionals: []string{"key"}, Required: []string{"key"}, Properties: props("key", str("Issue key or numeric ID."), "fields", str("Comma-separated Jira field IDs; default summary,status,assignee,issuetype,project,updated."))},
	{Name: "jira issue search", Description: "Search issues with JQL; return one bounded page.", Required: []string{"jql"}, Properties: paginated(props("jql", str("Jira Query Language expression."), "fields", str("Comma-separated field IDs; default summary,status,assignee,updated.")))},
	{Name: "jira issue create", Description: "Create an issue using native fields; discover required fields first.", Mutation: true, Required: []string{"fields"}, Properties: props("fields", obj("Jira fields object, including project, issuetype, summary; use ADF for rich text."))},
	{Name: "jira issue update", Description: "Update only the supplied issue fields.", Mutation: true, Positionals: []string{"key"}, Required: []string{"key", "fields"}, Properties: props("key", str("Issue key or ID."), "fields", obj("Jira fields to change. Null clears a field where supported."))},
	{Name: "jira issue transitions", Description: "Discover transitions and required transition fields.", Positionals: []string{"key"}, Required: []string{"key"}, Properties: props("key", str("Issue key or ID."))},
	{Name: "jira issue transition", Description: "Apply an available transition by ID; verifies it before writing.", Mutation: true, Positionals: []string{"key"}, Required: []string{"key", "transition-id"}, Properties: props("key", str("Issue key or ID."), "transition-id", str("ID returned by transitions."), "fields", obj("Values for transition screen fields, including required fields."))},
	{Name: "jira issue edit-fields", Description: "Discover editable fields for an existing issue.", Positionals: []string{"key"}, Required: []string{"key"}, Properties: props("key", str("Issue key or ID."))},
	{Name: "jira comment list", Description: "Read a bounded page of issue comments.", Positionals: []string{"key"}, Required: []string{"key"}, Properties: paginated(props("key", str("Issue key or ID.")))},
	{Name: "jira comment get", Description: "Read a specific comment; --raw includes its ADF body.", Positionals: []string{"key", "comment-id"}, Required: []string{"key", "comment-id"}, Properties: props("key", str("Issue key or ID."), "comment-id", str("Numeric comment ID."))},
	{Name: "jira comment add", Description: "Add a Markdown, plain-text, or native ADF comment.", Mutation: true, Positionals: []string{"key"}, Required: []string{"key"}, Properties: commentProperties(false)},
	{Name: "jira comment update", Description: "Replace one comment after checking its last-read updated timestamp.", Mutation: true, Positionals: []string{"key", "comment-id"}, Required: []string{"key", "comment-id", "expected-updated"}, Properties: commentProperties(true)},
	{Name: "jira project list", Description: "List a bounded page of projects.", Properties: paginated(props())},
	{Name: "jira user assignable", Description: "Check a specific account's assignability in a project; an empty list means not verified.", Required: []string{"project", "account-id"}, Properties: props("project", str("Project key or ID."), "account-id", str("Exact Atlassian account ID to verify."))},
	{Name: "jira project issue-types", Description: "Discover issue types available for creation in a project.", Positionals: []string{"project"}, Required: []string{"project"}, Properties: paginated(props("project", str("Project key or ID.")))},
	{Name: "jira project create-fields", Description: "Discover required and allowed fields for issue creation.", Positionals: []string{"project"}, Required: []string{"project", "issue-type-id"}, Properties: paginated(props("project", str("Project key or ID."), "issue-type-id", str("Issue type ID returned by issue-types.")))},
	{Name: "confluence space list", Description: "List spaces, optionally filtered by space key.", Properties: paginated(props("key", str("Exact space key filter.")))},
	{Name: "confluence page list", Description: "List page metadata, optionally within a space.", Properties: paginated(props("space-id", str("Numeric space ID.")))},
	{Name: "confluence page get", Description: "Read page metadata and version; bodies are opt-in.", Positionals: []string{"id"}, Required: []string{"id"}, Properties: props("id", str("Numeric page ID."), "body-format", choice("Request a body only when needed.", "none", "none", "storage", "atlas_doc_format", "view"))},
	{Name: "confluence page search", Description: "Search with CQL; no excerpt/body by default.", Required: []string{"cql"}, Properties: paginated(props("cql", str("Confluence Query Language expression; use type=page for pages.")))},
	{Name: "confluence page children", Description: "List a bounded page of child pages.", Positionals: []string{"id"}, Required: []string{"id"}, Properties: paginated(props("id", str("Numeric parent page ID.")))},
	{Name: "confluence page create", Description: "Publish a page from Markdown or lossless storage XHTML.", Mutation: true, Required: []string{"space-id", "title"}, Properties: props("space-id", str("Numeric space ID from space list."), "title", str("Page title."), "parent-id", str("Optional parent page ID."), "body", str("Page content."), "body-file", str("UTF-8 content file, or - for stdin."), "format", choice("Input content format.", "markdown", "markdown", "storage"))},
	{Name: "confluence page update", Description: "Replace page content only if its version matches your read.", Mutation: true, Positionals: []string{"id"}, Required: []string{"id", "title", "expected-version"}, Properties: props("id", str("Numeric page ID."), "title", str("Page title; explicitly preserve or change it."), "expected-version", integer("Version returned by page get. Conflicts require a fresh read.", 0), "body", str("Replacement page content."), "body-file", str("UTF-8 content file, or - for stdin."), "format", choice("Use storage to preserve Confluence macros.", "markdown", "markdown", "storage"), "message", str("Optional version message."))},
	{Name: "confluence page delete", Description: "Move a current page to trash; requires --yes.", Mutation: true, Positionals: []string{"id"}, Required: []string{"id"}, Properties: props("id", str("Numeric page ID."), "yes", Property{Type: "boolean", Description: "Explicitly confirm this page deletion; ignored for dry-run."})},
}

func uploadProperties(product string) map[string]Property {
	p := props("file", str("Path to a readable regular file; no stdin. Streamed with a five-minute deadline; server size limits apply."), "filename", str("Optional uploaded basename; defaults to the local filename."), "content-type", str("Optional MIME type; inferred from filename or file bytes by default."))
	if product == "jira" {
		p["key"] = str("Issue key or numeric ID.")
	} else {
		p["id"] = str("Numeric page ID.")
		p["comment"] = str("Optional UTF-8 attachment version comment.")
		p["minor-edit"] = Property{Type: "boolean", Description: "Suppress attachment notification emails; default false."}
	}
	return p
}

func commentProperties(update bool) map[string]Property {
	p := props("key", str("Issue key or ID."), "body", str("Comment content; Markdown by default."), "body-file", str("UTF-8 content file, or - for stdin."), "format", choice("Input format; ignored for native --adf.", "markdown", "markdown", "text"), "adf", obj("Native Atlassian Document Format object; passed through unchanged."))
	if update {
		p["comment-id"] = str("Numeric comment ID.")
		p["expected-updated"] = str("Exact updated timestamp from comment get. Preflight check, not an atomic lock.")
	}
	return p
}

var globals = props(
	"input", str("JSON arguments as @file, - for stdin, or an inline object. Keys match flag names; flags override input."),
	"select", str("Comma-separated paths in each data record, e.g. key,summary,status. Up to 128 paths and 8192 bytes; preserves pagination metadata."),
	"raw", Property{Type: "boolean", Description: "Keep native Atlassian response data; may be large."},
	"pretty", Property{Type: "boolean", Description: "Indent JSON for human reading; compact JSON is the default."},
	"dry-run", Property{Type: "boolean", Description: "For mutations: validate local input and show the request without network access. Does not validate remote permissions or fields."},
)

func lookup(name string) *Command {
	for i := range commands {
		if commands[i].Name == name {
			return &commands[i]
		}
	}
	return nil
}

// Bound discovery by registered command depth, not by the number of arguments.
func commandDepth() int {
	depth := 0
	for _, command := range commands {
		depth = max(depth, strings.Count(command.Name, " ")+1)
	}
	return depth
}
