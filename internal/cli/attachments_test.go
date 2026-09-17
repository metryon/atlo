package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/metryon/atlo/internal/api"
)

func attachmentFile(t *testing.T, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "résumé.bin")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAttachmentMultipartAndCompactOutput(t *testing.T) {
	content := []byte{0, 1, '\r', '\n', 255, 254, 0, 'x'}
	path := attachmentFile(t, content)
	for _, product := range []string{"jira", "confluence"} {
		t.Run(product, func(t *testing.T) {
			name := product + " attachment upload"
			target, endpoint, token := "ENG-123", "/rest/api/3/issue/ENG-123/attachments", "no-check"
			response := `[{"id":"12","filename":"résumé.bin","size":8,"mimeType":"application/octet-stream","content":"https://site/attachment/12","author":{"large":"ignored"}}]`
			if product == "confluence" {
				target, endpoint, token = "123", "/wiki/rest/api/content/123/child/attachment", "nocheck"
				response = `{"results":[{"id":"12","title":"résumé.bin","extensions":{"fileSize":8,"mediaType":"application/octet-stream"},"version":{"number":1},"_links":{"download":"/download/attachments/123/résumé.bin"},"body":{"large":"ignored"}}]}`
			}
			args := []string{target, "--file", path, "--content-type", "application/octet-stream"}
			if product == "confluence" {
				args = append(args, "--comment", "Pièce jointe ✓", "--minor-edit")
			}
			a := parsed(t, name, args...)
			r, err := build(*lookup(name), a, strings.NewReader(""))
			if err != nil {
				t.Fatal(err)
			}
			defer r.Upload.Close()
			calls := 0
			c := mockClient(func(req *http.Request) (*http.Response, error) {
				calls++
				defer req.Body.Close()
				if req.Method != "POST" || req.URL.Path != endpoint || req.Header.Get("X-Atlassian-Token") != token {
					t.Fatalf("wrong request: %v", req)
				}
				if email, secret, ok := req.BasicAuth(); !ok || email != "e" || secret != "t" {
					t.Fatal("missing authentication")
				}
				encoded, err := io.ReadAll(req.Body)
				if err != nil || int64(len(encoded)) != req.ContentLength {
					t.Fatal("incorrect content length", err)
				}
				kind, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
				if err != nil || kind != "multipart/form-data" {
					t.Fatal("incorrect content type", err)
				}
				reader := multipart.NewReader(bytes.NewReader(encoded), params["boundary"])
				fields := map[string]string{}
				files := 0
				for {
					part, err := reader.NextPart()
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
					data, err := io.ReadAll(part)
					if err != nil {
						t.Fatal(err)
					}
					if part.FormName() == "file" {
						files++
						if part.FileName() != "résumé.bin" || part.Header.Get("Content-Type") != "application/octet-stream" || !bytes.Equal(data, content) {
							t.Fatal("file corrupted")
						}
					} else {
						if part.Header.Get("Content-Type") != "text/plain; charset=utf-8" {
							t.Fatal("missing UTF-8")
						}
						fields[part.FormName()] = string(data)
					}
				}
				if files != 1 {
					t.Fatal("expected exactly one file")
				}
				if product == "confluence" && (len(fields) != 2 || fields["minorEdit"] != "true" || fields["comment"] != "Pièce jointe ✓") {
					t.Fatal(fields)
				}
				if product == "jira" && len(fields) != 0 {
					t.Fatal(fields)
				}
				return resp(response), nil
			})
			result, err := perform(context.Background(), c, *lookup(name), a, r)
			if err != nil || calls != 1 {
				t.Fatal(calls, err)
			}
			items := result.Data.([]any)
			item := asMap(items[0])
			if item["id"] != "12" || item["filename"] != "résumé.bin" || number(item["size"]) != 8 || item["mimeType"] != "application/octet-stream" || item["author"] != nil || item["body"] != nil {
				t.Fatal(item)
			}
		})
	}
}

func TestAttachmentDryRunAndValidation(t *testing.T) {
	path := attachmentFile(t, []byte("SECRET FILE CONTENT MUST NOT APPEAR"))
	for _, product := range []string{"jira", "confluence"} {
		target := "ENG-123"
		if product == "confluence" {
			target = "123"
		}
		code, out, stderr := invoke([]string{product, "attachment", "upload", target, "--file", path, "--filename", "report.txt", "--dry-run"}, "")
		if code != 0 || stderr != "" || strings.Contains(out, "SECRET") {
			t.Fatal(code, out, stderr)
		}
		var envelope Envelope
		if err := json.Unmarshal([]byte(out), &envelope); err != nil {
			t.Fatal(err)
		}
		upload := asMap(asMap(envelope.Data)["upload"])
		if upload["filename"] != "report.txt" || !strings.HasPrefix(stringValue(upload["content_type"]), "text/plain") || number(upload["size"]) != len("SECRET FILE CONTENT MUST NOT APPEAR") {
			t.Fatal(upload)
		}
		if product == "confluence" && asMap(upload["fields"])["minorEdit"] != "false" {
			t.Fatal(upload)
		}
	}
	for _, args := range [][]string{
		{"--file", "-"}, {"--file", filepath.Dir(path)}, {"--file", path + "missing"},
		{"--file", path, "--filename", "../oops"}, {"--file", path, "--filename", "bad\r\nHeader"},
		{"--file", path, "--filename", "bad\\path"}, {"--file", path, "--content-type", "nonsense"},
		{"--file", path, "--content-type", "text/plain\r\nX: y"},
	} {
		code, out, stderr := invoke(append([]string{"jira", "attachment", "upload", "ENG-123", "--dry-run"}, args...), "")
		if code != 2 || out != "" || !json.Valid([]byte(stderr)) {
			t.Fatal(args, code, out, stderr)
		}
	}
}

func TestUploadLargerThanJSONLimitAndNoReplay(t *testing.T) {
	path := attachmentFile(t, nil)
	if err := os.Truncate(path, api.MaxBytes+1024); err != nil {
		t.Fatal(err)
	}
	upload, err := api.OpenUpload(path, "large.bin", "")
	if err != nil {
		t.Fatal(err)
	}
	defer upload.Close()
	for _, status := range []int{200, 429, 503, 413, 302, 0} {
		calls := 0
		c := mockClient(func(req *http.Request) (*http.Response, error) {
			calls++
			defer req.Body.Close()
			size, err := io.Copy(io.Discard, req.Body)
			if err != nil || size != req.ContentLength || size <= api.MaxBytes {
				t.Fatal(size, err)
			}
			if status == 0 {
				return nil, errors.New("connection lost")
			}
			r := resp(`[{"id":"1"}]`)
			r.StatusCode = status
			return r, nil
		})
		_, err := c.UploadFile(context.Background(), "/rest/api/3/issue/ENG-123/attachments", "jira", upload)
		if calls != 1 || (err == nil) != (status == 200) {
			t.Fatal(status, calls, err)
		}
		if err != nil && err.(*api.Error).Retryable {
			t.Fatal("upload marked retryable")
		}
		if status == 0 && !strings.Contains(err.Error(), "unknown") {
			t.Fatal(err)
		}
		if status == 413 && err.(*api.Error).Code != "payload_too_large" {
			t.Fatal(err)
		}
	}
}

func TestConfluenceAttachmentListPagination(t *testing.T) {
	name := "confluence attachment list"
	a := parsed(t, name, "123", "--limit", "1", "--cursor", "one")
	r, err := build(*lookup(name), a, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	c := mockClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/wiki/api/v2/pages/123/attachments" || req.URL.Query().Get("limit") != "1" || req.URL.Query().Get("cursor") != "one" {
			t.Fatal(req.URL)
		}
		res := resp(`{"results":[{"id":"att12","title":"report.pdf","fileSize":42,"mediaType":"application/pdf","downloadLink":"/download/report.pdf"}]}`)
		res.Header.Set("Link", `</wiki/api/v2/pages/123/attachments?cursor=two>; rel="next"`)
		return res, nil
	})
	result, err := perform(context.Background(), c, *lookup(name), a, r)
	if err != nil || result.Meta["next_cursor"] != "two" || result.Meta["count"] != 1 {
		t.Fatal(result, err)
	}
	item := asMap(result.Data.([]any)[0])
	if item["id"] != "att12" || number(item["size"]) != 42 || item["download"] != "/download/report.pdf" {
		t.Fatal(item)
	}
}

func TestUploadRequiresReceiptBeforeReportingSuccess(t *testing.T) {
	path := attachmentFile(t, nil)
	for _, product := range []string{"jira", "confluence"} {
		name := product + " attachment upload"
		a := parsed(t, name, "123", "--file", path)
		r, err := build(*lookup(name), a, strings.NewReader(""))
		if err != nil {
			t.Fatal(err)
		}
		for _, response := range []string{"", "[]", `{}`, `[{"filename":"missing-id"}]`, `[{"id":""}]`, `[{"id":123}]`} {
			c := mockClient(func(req *http.Request) (*http.Response, error) { req.Body.Close(); return resp(response), nil })
			_, err := perform(context.Background(), c, *lookup(name), a, r)
			if err == nil || err.(*api.Error).Code != "invalid_response" {
				t.Fatal(response, err)
			}
		}
		r.Upload.Close()
	}
}
