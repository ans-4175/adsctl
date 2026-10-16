package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"adsctl/internal/output"
)

func TestUploadNonJSON413Envelope(t *testing.T) {
	calls := 0
	a, out, errOut, cfg := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = r.Body.Read(make([]byte, 1024))
		w.WriteHeader(413)
		_, _ = fmt.Fprint(w, "proxy rejected fixture-token access_token=private-do-not-print")
	})
	file := filepath.Join(t.TempDir(), "video.mp4")
	if err := os.WriteFile(file, []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	err := executeTest(a, []string{"--config", cfg, "meta", "videos", "upload", "--profile", "test", "--account", "123", "--file", file, "--mode", "single"})
	if err == nil || output.AsError(err).Exit != 8 || calls != 1 {
		t.Fatal(err, calls)
	}
	var env output.Envelope
	if err = json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	data, _ := env.Data.(map[string]any)
	if env.OK || env.Meta["complete"] != false || env.Error.HTTPStatus != 413 || data["state"] != "failed" || env.Error.SuggestedAction == "" {
		t.Fatal(out.String())
	}
	for _, secret := range []string{"fixture-token", "private-do-not-print", "proxy rejected"} {
		if bytes.Contains(out.Bytes(), []byte(secret)) || bytes.Contains(errOut.Bytes(), []byte(secret)) {
			t.Fatal("unsafe response emitted")
		}
	}
}

func TestVideoDryRunModeAndLimits(t *testing.T) {
	a, out, _, cfg := testApp(t, func(w http.ResponseWriter, r *http.Request) { t.Error("dry run made a request") })
	file := filepath.Join(t.TempDir(), "video.mp4")
	if err := os.WriteFile(file, []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"auto", "resumable", "single"} {
		out.Reset()
		err := executeTest(a, []string{"--config", cfg, "meta", "videos", "upload", "--profile", "test", "--account", "123", "--file", file, "--mode", mode, "--dry-run"})
		expected := mode
		if expected == "auto" {
			expected = "resumable"
		}
		if err != nil || !bytes.Contains(out.Bytes(), []byte(`"mode": "`+expected+`"`)) || !bytes.Contains(out.Bytes(), []byte(`"max_file_size": 1073741824`)) {
			t.Fatal(err, out.String())
		}
	}
	if err := os.Truncate(file, 1<<30+1); err != nil {
		t.Fatal(err)
	}
	if err := executeTest(a, []string{"--config", cfg, "meta", "videos", "upload", "--profile", "test", "--account", "123", "--file", file, "--dry-run"}); err == nil {
		t.Fatal("oversized file accepted")
	}
}
