package graph

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadStreamsMultipartWithHeaderCredentials(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v25.0/act_123/adimages" || r.Header.Get("Authorization") != "Bearer fixture" || r.URL.RawQuery != "" {
			t.Error("wrong scope or auth")
		}
		m, err := r.MultipartReader()
		if err != nil {
			t.Error(err)
			return
		}
		part, err := m.NextPart()
		if err != nil {
			t.Error(err)
			return
		}
		b, _ := io.ReadAll(part)
		if part.FormName() != "filename" || string(b) != "imagefixture" {
			t.Error(part.FormName(), string(b))
		}
		_, _ = fmt.Fprint(w, `{"images":{"image.jpg":{"hash":"hash"}}}`)
	}))
	defer s.Close()
	path := filepath.Join(t.TempDir(), "image.jpg")
	if err := os.WriteFile(path, []byte("imagefixture"), 0600); err != nil {
		t.Fatal(err)
	}
	c := New("fixture", "v25.0")
	c.BaseURL = s.URL
	response, err := c.Upload(context.Background(), "act_123", "images", path)
	if err != nil || response.Value["images"] == nil {
		t.Fatal(err, response)
	}
}
