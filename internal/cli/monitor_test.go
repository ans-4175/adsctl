package cli

import (
	"adsctl/internal/output"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMonitorPreservesFindingWhenEvidenceUnavailable(t *testing.T) {
	status := "DISAPPROVED"
	missing := false
	a, out, _, cfg := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		if missing {
			_, _ = fmt.Fprint(w, `{"data":[{"id":"456"}]}`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"data":[{"id":"456","effective_status":%q}]}`, status)
	})
	dir := t.TempDir()
	file := filepath.Join(dir, "rules.yaml")
	if err := os.WriteFile(file, []byte("version: 1\nid: daily\nrules:\n  - id: rejected\n    type: delivery-status\n    resource: ads\n    effective_status_in: [DISAPPROVED]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--config", cfg, "meta", "monitor", "check", "--file", file, "--profile", "test", "--account", "123", "--state-dir", dir, "--fail-on-finding"}
	if err := executeTest(a, args); err == nil || output.AsError(err).Exit != 11 || !strings.Contains(out.String(), `"transition": "started"`) {
		t.Fatal(err, out.String())
	}
	missing = true
	out.Reset()
	if err := executeTest(a, args); err == nil || output.AsError(err).Exit != 9 {
		t.Fatal(err, out.String())
	}
	missing = false
	out.Reset()
	if err := executeTest(a, args); err == nil || !strings.Contains(out.String(), `"transition": "ongoing"`) {
		t.Fatal(err, out.String())
	}
	status = "ACTIVE"
	out.Reset()
	if err := executeTest(a, args); err != nil || !strings.Contains(out.String(), `"transition": "recovered"`) {
		t.Fatal(err, out.String())
	}
}
