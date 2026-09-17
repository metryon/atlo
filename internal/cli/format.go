package cli

import (
	"strings"
)

func asMap(v any) map[string]any { m, _ := v.(map[string]any); return m }
func firstString(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
func pick(m map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	for _, k := range keys {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	return out
}
func mapList(v any, fn func(map[string]any) any) []any {
	list, _ := v.([]any)
	out := make([]any, 0, len(list))
	for _, item := range list {
		out = append(out, fn(asMap(item)))
	}
	return out
}
func issueSummary(m map[string]any) any {
	out := pick(m, "key", "id")
	f := asMap(m["fields"])
	for k, v := range f {
		switch k {
		case "status", "issuetype":
			out[k] = asMap(v)["name"]
		case "assignee":
			if v == nil {
				out[k] = nil
			} else {
				out[k] = pick(asMap(v), "accountId", "displayName")
			}
		case "project":
			out[k] = pick(asMap(v), "id", "key", "name")
		default:
			out[k] = v
		}
	}
	return out
}
func pageSummary(m map[string]any, body bool) any {
	out := pick(m, "id", "title", "status", "spaceId", "parentId")
	if v := asMap(m["version"])["number"]; v != nil {
		out["version"] = v
	}
	if u := asMap(m["_links"])["webui"]; u != nil {
		out["url"] = u
	}
	if body && m["body"] != nil {
		out["body"] = m["body"]
	}
	return out
}
func fieldSummary(m map[string]any) any {
	return pick(m, "fieldId", "key", "name", "required", "schema", "allowedValues", "hasDefaultValue", "defaultValue", "operations", "autoCompleteUrl")
}
func compact(name string, data any, a Args) any {
	m := asMap(data)
	switch name {
	case "jira attachment upload":
		return mapList(data, func(m map[string]any) any { return pick(m, "id", "filename", "size", "mimeType", "content") })
	case "confluence attachment upload", "confluence attachment list":
		return mapList(m["results"], confluenceAttachmentSummary)
	case "jira user assignable":
		return mapList(data, func(user map[string]any) any { return pick(user, "accountId", "displayName", "active") })
	case "auth check":
		return pick(m, "accountId", "displayName", "active", "type")
	case "jira issue get", "jira issue create":
		return issueSummary(m)
	case "jira issue search":
		return mapList(m["issues"], issueSummary)
	case "jira issue transitions":
		return mapList(m["transitions"], func(t map[string]any) any { return pick(t, "id", "name", "to", "fields") })
	case "jira issue edit-fields":
		out := map[string]any{}
		for k, v := range asMap(m["fields"]) {
			out[k] = fieldSummary(asMap(v))
		}
		return out
	case "jira project list":
		return mapList(m["values"], func(p map[string]any) any { return pick(p, "id", "key", "name", "projectTypeKey") })
	case "jira project issue-types":
		return mapList(m["issueTypes"], func(p map[string]any) any { return pick(p, "id", "name", "subtask", "hierarchyLevel") })
	case "jira project create-fields":
		return mapList(m["fields"], fieldSummary)
	case "jira comment list":
		text := wantsField(a.S("select"), "text")
		return mapList(m["comments"], func(m map[string]any) any { return commentSummaryFields(m, text) })
	case "jira comment get":
		return commentSummaryFields(m, wantsField(a.S("select"), "text"))
	case "jira comment add", "jira comment update":
		return pick(m, "id", "created", "updated")
	case "confluence space list":
		return mapList(m["results"], func(p map[string]any) any { return pick(p, "id", "key", "name", "type", "status") })
	case "confluence page get":
		return pageSummary(m, a.S("body-format") != "none")
	case "confluence page create", "confluence page update":
		return pageSummary(m, false)
	case "confluence page list", "confluence page children":
		return mapList(m["results"], func(p map[string]any) any { return pageSummary(p, false) })
	case "confluence page search":
		return mapList(m["results"], func(p map[string]any) any {
			out := asMap(pageSummary(asMap(p["content"]), false))
			if p["url"] != nil {
				out["url"] = p["url"]
			}
			return out
		})
	}
	return data
}

func confluenceAttachmentSummary(m map[string]any) any {
	out := map[string]any{"id": m["id"], "filename": m["title"]}
	for target, source := range map[string]string{"size": "fileSize", "mimeType": "mediaType"} {
		value := m[source]
		if value == nil {
			value = asMap(m["extensions"])[source]
		}
		if value != nil {
			out[target] = value
		}
	}
	if version := asMap(m["version"])["number"]; version != nil {
		out["version"] = version
	}
	if link := firstString(stringValue(m["downloadLink"]), stringValue(asMap(m["_links"])["download"])); link != "" {
		out["download"] = link
	}
	return out
}
func commentSummary(m map[string]any) any {
	return commentSummaryFields(m, true)
}

func commentSummaryFields(m map[string]any, includeText bool) any {
	out := map[string]any{"id": m["id"], "author": pick(asMap(m["author"]), "accountId", "displayName"), "created": m["created"], "updated": m["updated"]}
	if includeText {
		out["text"] = strings.TrimRight(adfPlain(m["body"]), "\n")
	}
	return out
}

// Text projection keeps comments inexpensive; --raw preserves the full ADF.
func adfPlain(value any) string {
	var b strings.Builder
	appendADFText(&b, value)
	return b.String()
}

func appendADFText(b *strings.Builder, value any) {
	m := asMap(value)
	kind, _ := m["type"].(string)
	if kind == "text" {
		s, _ := m["text"].(string)
		b.WriteString(s)
		return
	}
	if kind == "hardBreak" {
		b.WriteByte('\n')
		return
	}
	if kind == "mention" {
		b.WriteString(firstString(stringValue(asMap(m["attrs"])["text"]), "@"+stringValue(asMap(m["attrs"])["id"])))
		return
	}
	if kind == "inlineCard" || kind == "blockCard" {
		b.WriteString(stringValue(asMap(m["attrs"])["url"]))
		return
	}
	if kind == "emoji" {
		b.WriteString(firstString(stringValue(asMap(m["attrs"])["text"]), stringValue(asMap(m["attrs"])["shortName"])))
		return
	}
	children, _ := m["content"].([]any)
	for _, child := range children {
		appendADFText(b, child)
	}
	if kind == "paragraph" || kind == "heading" || kind == "codeBlock" || kind == "tableRow" {
		b.WriteByte('\n')
	}
	if kind == "tableCell" || kind == "tableHeader" {
		b.WriteByte('\t')
	}
	if len(children) == 0 && kind != "doc" && kind != "paragraph" {
		b.WriteString("[" + kind + "]")
	}
}
func stringValue(v any) string { s, _ := v.(string); return s }
