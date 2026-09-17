package api

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/metryon/atlo/internal/fileio"
)

const UploadTimeout = 5 * time.Minute

// Upload holds a validated, open regular file. JSON exposes only metadata.
// The caller owns Close, including when only previewing the request.
type Upload struct {
	Path        string            `json:"file"`
	Filename    string            `json:"filename"`
	Size        int64             `json:"size"`
	ContentType string            `json:"content_type"`
	Fields      map[string]string `json:"fields,omitempty"`
	file        *os.File
}

func OpenUpload(path, filename, contentType string) (*Upload, error) {
	invalid := func(message string) (*Upload, error) { return nil, &Error{Code: "validation", Message: message} }
	if path == "-" {
		return invalid("Uploads require a regular file path; stdin is not supported.")
	}
	if filename == "" {
		filename = filepath.Base(path)
	}
	if filename == "." || filename == ".." || !utf8.ValidString(filename) || strings.ContainsAny(filename, "/\\") || strings.IndexFunc(filename, unicode.IsControl) >= 0 {
		return invalid("Attachment filename must be a UTF-8 basename without control characters.")
	}
	if contentType != "" {
		kind, _, err := mime.ParseMediaType(contentType)
		if err != nil || !strings.Contains(kind, "/") || strings.IndexFunc(contentType, unicode.IsControl) >= 0 {
			return invalid("content-type must be a valid MIME media type.")
		}
	}
	f, err := fileio.OpenRegular(path)
	if err != nil {
		return invalid("Attachment file could not be opened for reading.")
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return invalid("Attachment must be a regular file.")
	}
	var sample [512]byte
	n, err := f.ReadAt(sample[:], 0)
	if err != nil && err != io.EOF {
		f.Close()
		return invalid("Attachment file could not be read.")
	}
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(filename))
		if contentType == "" {
			contentType = http.DetectContentType(sample[:n])
		}
	}
	return &Upload{Path: path, Filename: filename, Size: info.Size(), ContentType: contentType, file: f}, nil
}

func (u *Upload) Close() error { return u.file.Close() }

// UploadFile sends exactly once. Only multipart framing is buffered; file bytes
// are streamed from the open descriptor, beyond the JSON input size limit.
func (c *Client) UploadFile(ctx context.Context, path, product string, u *Upload) (Response, error) {
	var framing bytes.Buffer
	w := multipart.NewWriter(&framing)
	keys := make([]string, 0, len(u.Fields))
	for key := range u.Fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": key}))
		header.Set("Content-Type", "text/plain; charset=utf-8")
		part, err := w.CreatePart(header)
		if err != nil {
			return Response{}, err
		}
		if _, err := io.WriteString(part, u.Fields[key]); err != nil {
			return Response{}, err
		}
	}
	header := textproto.MIMEHeader{}
	// Quoted UTF-8 filenames match ordinary browser uploads (no filename*).
	quoted := strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(u.Filename)
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+quoted+`"`)
	header.Set("Content-Type", u.ContentType)
	if _, err := w.CreatePart(header); err != nil {
		return Response{}, err
	}
	prefixLen := framing.Len()
	if err := w.Close(); err != nil {
		return Response{}, err
	}
	frame := framing.Bytes()
	body := func() io.Reader {
		return io.MultiReader(bytes.NewReader(frame[:prefixLen]), io.NewSectionReader(u.file, 0, u.Size), bytes.NewReader(frame[prefixLen:]))
	}
	headers := http.Header{"Content-Type": {w.FormDataContentType()}}
	token := "no-check"
	if product == "confluence" {
		token = "nocheck"
	}
	headers.Set("X-Atlassian-Token", token)
	// Copy the clients so the longer deadline affects only this upload.
	uploadClient := *c
	httpClient := *c.HTTP
	httpClient.Timeout = UploadTimeout
	uploadClient.HTTP = &httpClient
	return uploadClient.do(ctx, "POST", path, body, u.Size+int64(len(frame)), headers)
}
