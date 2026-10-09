package graph

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"adsctl/internal/output"
)

// Upload streams a regular file once. Transport failure never retries a create.
func (c *Client) Upload(ctx context.Context, account, kind, path string) (Response, error) {
	if kind != "images" && kind != "videos" {
		return Response{}, output.Fail("USAGE", "Unsupported media type.", 2)
	}
	if err := c.pace(ctx); err != nil {
		return Response{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return Response{}, output.Fail("INPUT", "Cannot open upload file.", 2)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > 1<<30 {
		return Response{}, output.Fail("INPUT", "Upload requires a nonempty regular file at most 1 GiB.", 2)
	}
	base, err := url.Parse(c.BaseURL)
	if err != nil || base.Host == "" {
		return Response{}, output.Fail("CONFIG", "Invalid Graph base URL.", 2)
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/" + c.Version + "/" + account + map[string]string{"images": "/adimages", "videos": "/advideos"}[kind]
	reader, writer := io.Pipe()
	multi := multipart.NewWriter(writer)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if c.AppSecret != "" {
			h := hmac.New(sha256.New, []byte(c.AppSecret))
			_, _ = h.Write([]byte(c.Token))
			if e := multi.WriteField("appsecret_proof", hex.EncodeToString(h.Sum(nil))); e != nil {
				_ = writer.CloseWithError(e)
				return
			}
		}
		field := map[string]string{"images": "filename", "videos": "source"}[kind]
		part, e := multi.CreateFormFile(field, filepath.Base(path))
		if e == nil {
			_, e = io.Copy(part, f)
		}
		if e == nil {
			e = multi.Close()
		}
		_ = writer.CloseWithError(e)
	}()
	defer func() { _ = reader.Close(); <-done }()
	reqCtx, cancel := context.WithTimeout(ctx, c.RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, "POST", base.String(), reader)
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Content-Type", multi.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("User-Agent", "adsctl/0.1")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Response{}, output.Fail("MUTATION_UNKNOWN", "Upload interrupted; outcome is unknown. Reconcile before retrying.", 10)
	}
	defer func() { _ = resp.Body.Close() }()
	return c.readResponse("POST", resp)
}
