package launch

import (
	"adsctl/internal/output"
	"crypto/rand"
	"encoding/hex"
	"time"
)

type Checkpoint struct {
	State   string         `json:"state"`
	ID      string         `json:"id,omitempty"`
	Receipt map[string]any `json:"receipt,omitempty"`
	Error   *output.Error  `json:"error,omitempty"`
}
type Run struct {
	Version    int                   `json:"version"`
	ID         string                `json:"id"`
	Plan       Plan                  `json:"plan"`
	State      string                `json:"state"`
	UpdatedAt  string                `json:"updated_at"`
	Steps      map[string]Checkpoint `json:"steps"`
	Activation map[string]Checkpoint `json:"activation"`
	Decisions  []map[string]any      `json:"decisions,omitempty"`
}

func NewRun(p Plan) (Run, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return Run{}, err
	}
	r := Run{Version: 1, ID: "run_" + hex.EncodeToString(b), Plan: p, State: "pending", Steps: map[string]Checkpoint{}, Activation: map[string]Checkpoint{}}
	for _, s := range p.Steps {
		r.Steps[s.Key] = Checkpoint{State: "pending"}
	}
	r.Touch()
	return r, nil
}
func (r *Run) Touch() { r.UpdatedAt = time.Now().UTC().Format(time.RFC3339) }
func (r Run) IDs() map[string]string {
	out := map[string]string{}
	for k, s := range r.Steps {
		if s.ID != "" {
			out[k] = s.ID
		}
	}
	return out
}
func (r Run) Validate() error {
	if r.Version != 1 || r.ID == "" || r.Activation == nil || len(r.Steps) != len(r.Plan.Steps) {
		return output.Fail("JOURNAL_INVALID", "Invalid launch journal.", 2)
	}
	if err := r.Plan.Validate(true); err != nil {
		return err
	}
	for _, s := range r.Plan.Steps {
		cp, ok := r.Steps[s.Key]
		if !ok {
			return output.Fail("JOURNAL_INVALID", "Missing step checkpoint.", 2)
		}
		switch cp.State {
		case "pending", "failed", "in_flight", "unknown", "applied_unverified", "verified":
		default:
			return output.Fail("JOURNAL_INVALID", "Unknown checkpoint state.", 2)
		}
	}
	return nil
}
func (r Run) Ready() bool {
	for _, s := range r.Plan.Steps {
		if r.Steps[s.Key].State != "verified" || r.Steps[s.Key].ID == "" {
			return false
		}
	}
	return true
}
