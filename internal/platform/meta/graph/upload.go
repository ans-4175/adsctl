package graph

import (
	"bytes"
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
	f, err := os.Open(path)
	if err != nil {
		return Response{}, output.Fail("INPUT", "Cannot open upload file.", 2)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > 1<<30 {
		return Response{}, output.Fail("INPUT", "Upload requires a nonempty regular file at most 1 GiB.", 2)
	}
	field := map[string]string{"images": "filename", "videos": "source"}[kind]
	return c.uploadMultipart(ctx, account, kind, field, path, f, info.Size(), nil)
}

func (c *Client) uploadMultipart(ctx context.Context, account, kind, field, path string, source io.Reader, size int64, params url.Values) (Response, error) {
	if err := c.pace(ctx); err != nil {
		return Response{}, err
	}
	base, err := url.Parse(c.BaseURL)
	if err != nil || base.Host == "" {
		return Response{}, output.Fail("CONFIG", "Invalid Graph base URL.", 2)
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/" + c.Version + "/" + account + map[string]string{"images": "/adimages", "videos": "/advideos"}[kind]
	// Buffer only the multipart framing. The file range stays streamed, and
	// Content-Length makes the request boundary explicit on every transport.
	var framing bytes.Buffer
	multi := multipart.NewWriter(&framing)
	if c.AppSecret != "" {
		h := hmac.New(sha256.New, []byte(c.AppSecret))
		_, _ = h.Write([]byte(c.Token))
		if err = multi.WriteField("appsecret_proof", hex.EncodeToString(h.Sum(nil))); err != nil {
			return Response{}, err
		}
	}
	for key, values := range params {
		for _, value := range values {
			if err = multi.WriteField(key, value); err != nil {
				return Response{}, err
			}
		}
	}
	if _, err = multi.CreateFormFile(field, filepath.Base(path)); err != nil {
		return Response{}, err
	}
	prefix := append([]byte(nil), framing.Bytes()...)
	framing.Reset()
	if err = multi.Close(); err != nil {
		return Response{}, err
	}
	suffix := framing.Bytes()
	body := io.MultiReader(bytes.NewReader(prefix), io.LimitReader(source, size), bytes.NewReader(suffix))
	reqCtx, cancel := context.WithTimeout(ctx, c.RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, "POST", base.String(), body)
	if err != nil {
		return Response{}, err
	}
	req.ContentLength = int64(len(prefix)+len(suffix)) + size
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
