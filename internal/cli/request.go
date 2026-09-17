package cli

import (
	"io"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/metryon/atlo/internal/api"
)

type request struct {
	Product string      `json:"product"`
	Method  string      `json:"method"`
	Path    string      `json:"path"`
	Body    any         `json:"body,omitempty"`
	Upload  *api.Upload `json:"upload,omitempty"`
}

func endpoint(path string, q url.Values) string {
	if len(q) > 0 {
		return path + "?" + q.Encode()
	}
	return path
}
func offset(a Args) (int, error) {
	if a.S("cursor") == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(a.S("cursor"))
	if err != nil || n < 0 || n > int(^uint(0)>>1)-a.N("limit") {
		return 0, invalid("Invalid pagination cursor for this command.")
	}
	return n, nil
}

func build(cmd Command, a Args, stdin io.Reader) (request, error) {
	r := request{Product: strings.Split(cmd.Name, " ")[0], Method: "GET"}
	q := url.Values{}
	if _, ok := cmd.Properties["limit"]; ok {
		q.Set("limit", strconv.Itoa(a.N("limit")))
		if a.S("cursor") != "" {
			q.Set("cursor", a.S("cursor"))
		}
	}
	issue := "/rest/api/3/issue/" + url.PathEscape(a.S("key"))
	page := "/wiki/api/v2/pages/" + url.PathEscape(a.S("id"))
	switch cmd.Name {
	case "jira attachment upload", "confluence attachment upload":
		if !utf8.ValidString(a.S("comment")) {
			return r, invalid("Attachment comment must be valid UTF-8.")
		}
		upload, err := api.OpenUpload(a.S("file"), a.S("filename"), a.S("content-type"))
		if err != nil {
			return r, err
		}
		r.Upload = upload
		r.Method = "POST"
		r.Path = issue + "/attachments"
		if r.Product == "confluence" {
			r.Path = "/wiki/rest/api/content/" + a.S("id") + "/child/attachment"
			upload.Fields = map[string]string{"minorEdit": strconv.FormatBool(a.B("minor-edit"))}
			if comment := a.S("comment"); comment != "" {
				upload.Fields["comment"] = comment
			}
		}
	case "confluence attachment list":
		r.Path = endpoint(page+"/attachments", q)
	case "jira user assignable":
		r.Path = endpoint("/rest/api/3/user/assignable/search", url.Values{"project": {a.S("project")}, "accountId": {a.S("account-id")}, "maxResults": {"1"}})
	case "auth check":
		r.Product = a.S("product")
		if r.Product == "jira" {
			r.Path = "/rest/api/3/myself"
		} else {
			r.Path = "/wiki/rest/api/user/current"
		}
	case "jira issue get":
		fields := a.S("fields")
		if fields == "" {
			fields = "summary,status,assignee,issuetype,project,updated"
		}
		r.Path = endpoint(issue, url.Values{"fields": {fields}})
	case "jira issue search":
		fields := a.S("fields")
		if fields == "" {
			fields = "summary,status,assignee,updated"
		}
		q = url.Values{"jql": {a.S("jql")}, "maxResults": {strconv.Itoa(a.N("limit"))}, "fields": {fields}}
		if a.S("cursor") != "" {
			q.Set("nextPageToken", a.S("cursor"))
		}
		r.Path = endpoint("/rest/api/3/search/jql", q)
	case "jira issue create", "jira issue update":
		fields := a["fields"].(map[string]any)
		if len(fields) == 0 {
			return r, invalid("fields must not be empty.")
		}
		r.Body = map[string]any{"fields": fields}
		r.Method = "PUT"
		r.Path = issue
		if cmd.Name == "jira issue create" {
			for _, name := range []string{"project", "issuetype", "summary"} {
				if fields[name] == nil {
					return r, invalid("fields.%s is required; discover create-fields first.", name)
				}
			}
			r.Method = "POST"
			r.Path = "/rest/api/3/issue"
		}
	case "jira issue transitions":
		r.Path = issue + "/transitions?expand=transitions.fields"
	case "jira issue transition":
		r.Method = "POST"
		r.Path = issue + "/transitions"
		r.Body = map[string]any{"transition": map[string]string{"id": a.S("transition-id")}}
		if fields, ok := a["fields"]; ok {
			r.Body.(map[string]any)["fields"] = fields
		}
	case "jira issue edit-fields":
		r.Path = issue + "/editmeta"
	case "jira comment get":
		r.Path = issue + "/comment/" + a.S("comment-id")
	case "jira comment add", "jira comment update":
		body, hasADF := a["adf"]
		if hasADF {
			if _, ok := a["body"]; ok {
				return r, invalid("adf and body are mutually exclusive.")
			}
			if _, ok := a["body-file"]; ok {
				return r, invalid("adf and body-file are mutually exclusive.")
			}
			m := body.(map[string]any)
			version, ok := integerValue(m["version"])
			if m["type"] != "doc" || !ok || version != 1 {
				return r, invalid("adf must be a version 1 doc.")
			}
			if _, ok := m["content"].([]any); !ok {
				return r, invalid("adf.content must be an array.")
			}
		} else {
			t, err := textBody(a, stdin)
			if err != nil {
				return r, err
			}
			if a.S("format") == "text" {
				body = adfText(t)
			} else {
				body, err = markdownADF(t)
				if err != nil {
					return r, err
				}
			}
		}
		r.Method = "POST"
		r.Path = issue + "/comment"
		r.Body = map[string]any{"body": body}
		if cmd.Name == "jira comment update" {
			r.Method = "PUT"
			r.Path += "/" + a.S("comment-id")
		}
	case "jira comment list", "jira project list", "jira project issue-types", "jira project create-fields":
		start, err := offset(a)
		if err != nil {
			return r, err
		}
		q = url.Values{"startAt": {strconv.Itoa(start)}, "maxResults": {strconv.Itoa(a.N("limit"))}}
		switch cmd.Name {
		case "jira comment list":
			r.Path = issue + "/comment"
		case "jira project list":
			r.Path = "/rest/api/3/project/search"
		case "jira project issue-types":
			r.Path = "/rest/api/3/issue/createmeta/" + url.PathEscape(a.S("project")) + "/issuetypes"
		case "jira project create-fields":
			r.Path = "/rest/api/3/issue/createmeta/" + url.PathEscape(a.S("project")) + "/issuetypes/" + a.S("issue-type-id")
		}
		r.Path = endpoint(r.Path, q)
	case "confluence space list":
		if a.S("key") != "" {
			q.Set("keys", a.S("key"))
		}
		r.Path = endpoint("/wiki/api/v2/spaces", q)
	case "confluence page list":
		if a.S("space-id") != "" {
			q.Set("space-id", a.S("space-id"))
		}
		r.Path = endpoint("/wiki/api/v2/pages", q)
	case "confluence page get":
		if a.S("body-format") != "none" {
			q.Set("body-format", a.S("body-format"))
		}
		r.Path = endpoint(page, q)
	case "confluence page search":
		q.Set("cql", a.S("cql"))
		r.Path = endpoint("/wiki/rest/api/search", q)
	case "confluence page children":
		r.Path = endpoint(page+"/children", q)
	case "confluence page create", "confluence page update":
		body, err := storageBody(a, stdin)
		if err != nil {
			return r, err
		}
		payload := map[string]any{"title": a.S("title"), "status": "current", "body": map[string]string{"representation": "storage", "value": body}}
		r.Body = payload
		if cmd.Name == "confluence page create" {
			r.Method = "POST"
			r.Path = "/wiki/api/v2/pages"
			payload["spaceId"] = a.S("space-id")
			if a.S("parent-id") != "" {
				payload["parentId"] = a.S("parent-id")
			}
		} else {
			r.Method = "PUT"
			r.Path = page
			payload["id"] = a.S("id")
			if a.N("expected-version") == int(^uint(0)>>1) {
				return r, invalid("expected-version is too large to increment.")
			}
			v := map[string]any{"number": a.N("expected-version") + 1}
			if a.S("message") != "" {
				v["message"] = a.S("message")
			}
			payload["version"] = v
		}
	case "confluence page delete":
		if !a.B("yes") && !a.B("dry-run") {
			return r, invalid("Deleting a page requires --yes.")
		}
		r.Method = "DELETE"
		r.Path = page
	default:
		return r, invalid("Unsupported command.")
	}
	return r, nil
}
