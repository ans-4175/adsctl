package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"adsctl/internal/output"
)

func TestMutationDefaultsPausedAndVerifies(t *testing.T) {
	posts := 0
	a, out, _, path := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts++
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.Form.Get("status") != "PAUSED" || r.Form.Get("special_ad_categories") != "[]" {
				t.Errorf("unsafe defaults: %v", r.Form)
			}
			_, _ = fmt.Fprint(w, `{"id":"456"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"id":"456","account_id":"123","status":"PAUSED","effective_status":"PAUSED"}`)
	})
	err := executeTest(a, []string{"--config", path, "meta", "campaigns", "create", "--profile", "test", "--account", "123", "--name", "test", "--objective", "OUTCOME_TRAFFIC", "--special-ad-categories", "NONE"})
	if err != nil || posts != 1 || !bytes.Contains(out.Bytes(), []byte(`"state": "verified"`)) {
		t.Fatal(err, posts, out.String())
	}
}
func TestWritesRequireExplicitScopeAndDryRunDoesNotWrite(t *testing.T) {
	t.Setenv("ADSCTL_PROFILE", "test")
	for _, dry := range []bool{false, true} {
		posts := 0
		a, out, _, path := testApp(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				posts++
			}
			_, _ = fmt.Fprint(w, `{"id":"456","account_id":"123"}`)
		})
		args := []string{"--config", path, "meta", "campaigns", "pause", "456", "--account", "123"}
		if dry {
			args = append(args, "--profile", "test", "--dry-run")
		}
		err := executeTest(a, args)
		if posts != 0 {
			t.Fatal("unexpected write")
		}
		if dry {
			if err != nil || !bytes.Contains(out.Bytes(), []byte(`"state": "planned"`)) {
				t.Fatal(err, out.String())
			}
		} else if err == nil || output.AsError(err).Code != "WRITE_SCOPE" {
			t.Fatal(err)
		}
	}
}
func TestAmbiguousCreateIsNotRetried(t *testing.T) {
	posts := 0
	a, out, _, path := testApp(t, func(w http.ResponseWriter, r *http.Request) { posts++; _, _ = fmt.Fprint(w, `unreadable`) })
	err := executeTest(a, []string{"--config", path, "meta", "campaigns", "create", "--profile", "test", "--account", "123", "--name", "test", "--objective", "OUTCOME_TRAFFIC", "--special-ad-categories", "NONE"})
	if err == nil || output.AsError(err).Exit != 10 || posts != 1 || !bytes.Contains(out.Bytes(), []byte(`"state": "unknown"`)) {
		t.Fatal(err, posts, out.String())
	}
}
func TestCrossAccountWriteBlocked(t *testing.T) {
	posts := 0
	a, _, _, path := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			posts++
		}
		_, _ = fmt.Fprint(w, `{"id":"456","account_id":"999"}`)
	})
	err := executeTest(a, []string{"--config", path, "meta", "campaigns", "activate", "456", "--profile", "test", "--account", "123"})
	if err == nil || output.AsError(err).Exit != 4 || posts != 0 {
		t.Fatal(err, posts)
	}
}
func TestVerificationFailurePreservesAppliedReceipt(t *testing.T) {
	a, out, _, path := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			_, _ = fmt.Fprint(w, `{"id":"456"}`)
		} else {
			_, _ = fmt.Fprint(w, `{"error":{"code":100,"message":"unavailable field"}}`)
		}
	})
	err := executeTest(a, []string{"--config", path, "meta", "campaigns", "create", "--profile", "test", "--account", "123", "--name", "test", "--objective", "OUTCOME_TRAFFIC", "--special-ad-categories", "NONE"})
	if err == nil || output.AsError(err).Exit != 9 || !bytes.Contains(out.Bytes(), []byte(`"id": "456"`)) || !bytes.Contains(out.Bytes(), []byte(`"state": "applied_unverified"`)) {
		t.Fatal(err, out.String())
	}
}
