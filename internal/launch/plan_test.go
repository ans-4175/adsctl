package launch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const manifest = `version: 1
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
      daily_budget: 9007199254740993
      billing_event: IMPRESSIONS
      optimization_goal: LINK_CLICKS
      targeting:
        geo_locations:
          countries: [ID]
creatives:
  - key: post
    payload:
      name: Existing post
      object_story_id: '123_456'
ads:
  - key: ad
    adset_ref: audience
    creative_ref: post
    payload:
      name: Ad
`

func TestPlanResolvesReferencesAndPreservesIntegers(t *testing.T) {
	p, err := Compile([]byte(manifest), t.TempDir(), Scope{Account: "act_123"})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Validate(true); err != nil {
		t.Fatal(err)
	}
	if p.Steps[0].Node.Payload["status"] != "PAUSED" {
		t.Fatal(p)
	}
	payload, err := Payload(p.Steps[1], map[string]string{"campaigns.main": "999"})
	if err != nil || payload["campaign_id"] != "999" || payload["daily_budget"].(interface{ String() string }).String() != "9007199254740993" {
		t.Fatal(err, payload)
	}
	p.Steps[0].Node.Payload["name"] = "changed"
	if p.Validate(true) == nil {
		t.Fatal("modified plan accepted")
	}
}
func TestPlanRejectsConflictsAndActiveCreates(t *testing.T) {
	for _, bad := range []string{
		strings.Replace(manifest, "campaign_ref: main", "campaign_ref: missing", 1),
		strings.Replace(manifest, "name: Audience", "name: Audience\n      campaign_id: '555'", 1),
		strings.Replace(manifest, "name: Traffic", "name: Traffic\n      status: ACTIVE", 1),
		strings.Replace(manifest, "creative_ref: post", "creative_ref: audience", 1),
		strings.Replace(manifest, "version: 1", "version: 2", 1),
	} {
		if _, err := Compile([]byte(bad), t.TempDir(), Scope{}); err == nil {
			t.Fatal("accepted", bad)
		}
	}
}
func TestPlanDetectsChangedAssets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "banner.jpg")
	if err := os.WriteFile(path, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	raw := strings.Replace(manifest, "campaigns:", "assets:\n  - key: banner\n    type: image\n    file: banner.jpg\ncampaigns:", 1)
	p, err := Compile([]byte(raw), dir, Scope{})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.CheckFiles(); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("after"), 0600); err != nil {
		t.Fatal(err)
	}
	if p.CheckFiles() == nil {
		t.Fatal("changed media accepted")
	}
}
