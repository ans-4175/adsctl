package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"adsctl/internal/launch"
	"adsctl/internal/output"
	"adsctl/internal/state"
)

const launchFixture = `version: 1
kind: meta-launch
name: traffic
campaigns:
  - key: main
    payload:
      name: Traffic
      objective: OUTCOME_TRAFFIC
      special_ad_categories: []
adsets:
  - key: audience
    campaign_ref: main
    payload:
      name: Audience
      daily_budget: 2500
      billing_event: IMPRESSIONS
      optimization_goal: LINK_CLICKS
      targeting:
        geo_locations:
          countries: [ID]
creatives:
  - key: post
    payload:
      name: Post
      object_story_id: '123_456'
ads:
  - key: ad
    adset_ref: audience
    creative_ref: post
    payload:
      name: Ad
`

type launchServer struct {
	mu         sync.Mutex
	objects    map[string]map[string]any
	creates    int
	activation []string
	ambiguous  bool
}

func (s *launchServer) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/v25.0/")
	if path == "act_123" {
		_, _ = fmt.Fprint(w, `{"id":"act_123","currency":"USD"}`)
		return
	}
	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(400)
			return
		}
		if strings.Contains(path, "/") {
			s.creates++
			id := fmt.Sprint(100 + s.creates)
			obj := map[string]any{"id": id, "account_id": "123"}
			for key, vs := range r.Form {
				var val any
				d := json.NewDecoder(strings.NewReader(vs[0]))
				d.UseNumber()
				if err := d.Decode(&val); err != nil || !json.Valid([]byte(vs[0])) {
					val = vs[0]
				}
				obj[key] = val
			}
			s.objects[id] = obj
			if s.ambiguous {
				s.ambiguous = false
				w.WriteHeader(500)
				_, _ = fmt.Fprint(w, `{"unavailable":true}`)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
			return
		}
		obj := s.objects[path]
		for key, values := range r.Form {
			obj[key] = values[0]
		}
		if obj["status"] == "ACTIVE" {
			s.activation = append(s.activation, path)
		}
		_, _ = fmt.Fprint(w, `{"success":true}`)
		return
	}
	obj := s.objects[path]
	if obj == nil {
		_, _ = fmt.Fprint(w, `{"error":{"code":100,"message":"unknown"}}`)
		return
	}
	_ = json.NewEncoder(w).Encode(obj)
}
func launchFiles(t *testing.T) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	manifest := filepath.Join(dir, "launch.yaml")
	plan := filepath.Join(dir, "plan.json")
	if err := os.WriteFile(manifest, []byte(launchFixture), 0600); err != nil {
		t.Fatal(err)
	}
	return manifest, plan, filepath.Join(dir, "state")
}
func TestLaunchFullFlowAndDuplicatePlanGuard(t *testing.T) {
	server := &launchServer{objects: map[string]map[string]any{}}
	a, out, _, cfg := testApp(t, server.serve)
	manifest, plan, dir := launchFiles(t)
	exec := func(args ...string) error {
		out.Reset()
		base := []string{"--config", cfg, "meta", "launch"}
		base = append(base, args...)
		base = append(base, "--profile", "test", "--account", "123", "--state-dir", dir)
		return executeTest(a, base)
	}
	if err := exec("plan", "--file", manifest, "--save", plan); err != nil {
		t.Fatal(err)
	}
	if server.creates != 0 {
		t.Fatal("plan mutated")
	}
	if err := exec("apply", "--plan", plan); err != nil {
		t.Fatal(err, out.String())
	}
	if server.creates != 4 {
		t.Fatal(server.creates)
	}
	var env struct {
		Data launch.Run `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	run := env.Data
	if run.State != "created_paused" || !run.Ready() {
		t.Fatal(run)
	}
	if err := exec("apply", "--plan", plan); err == nil || output.AsError(err).Code != "RUN_EXISTS" || server.creates != 4 {
		t.Fatal("duplicate apply", err, server.creates)
	}
	if err := exec("resume", run.ID); err != nil || server.creates != 4 {
		t.Fatal("resume duplicated", err)
	}
	if err := exec("activate", run.ID, "--dry-run"); err != nil || len(server.activation) != 0 || !bytes.Contains(out.Bytes(), []byte(`"state": "planned"`)) {
		t.Fatal(err, out.String())
	}
	if err := exec("activate", run.ID); err != nil {
		t.Fatal(err, out.String())
	}
	if strings.Join(server.activation, ",") != "104,102,101" {
		t.Fatal("activation order", server.activation)
	}
	if err := exec("activate", run.ID); err != nil || len(server.activation) != 3 {
		t.Fatal("repeat activation", err, server.activation)
	}
}
func TestUnknownLaunchRequiresExplicitReconciliation(t *testing.T) {
	server := &launchServer{objects: map[string]map[string]any{}, ambiguous: true}
	a, out, _, cfg := testApp(t, server.serve)
	manifest, plan, dir := launchFiles(t)
	exec := func(args ...string) error {
		out.Reset()
		base := append([]string{"--config", cfg, "meta", "launch"}, args...)
		return executeTest(a, append(base, "--profile", "test", "--account", "123", "--state-dir", dir))
	}
	if err := exec("plan", "--file", manifest, "--save", plan); err != nil {
		t.Fatal(err)
	}
	if err := exec("apply", "--plan", plan); err == nil || output.AsError(err).Exit != 10 {
		t.Fatal(err)
	}
	var env struct {
		Data launch.Run `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	id := env.Data.ID
	if err := exec("resume", id); err == nil || output.AsError(err).Exit != 10 || server.creates != 1 {
		t.Fatal(err, server.creates)
	}
	if err := exec("reconcile", id, "--step", "campaigns.main", "--remote-id", "101"); err != nil {
		t.Fatal(err, out.String())
	}
	if err := exec("resume", id); err != nil || server.creates != 4 {
		t.Fatal(err, server.creates, out.String())
	}
	path, _ := state.Path(dir, id)
	var saved launch.Run
	if err := state.Read(path, &saved); err != nil || !saved.Ready() || len(saved.Decisions) != 1 {
		t.Fatal(err, saved)
	}
	// Local journal reads do not need credentials or profile flags.
	out.Reset()
	if err := executeTest(a, []string{"meta", "launch", "status", id, "--state-dir", dir}); err != nil {
		t.Fatal(err)
	}
}
