package insights

import (
	"testing"
)

func TestQueryValidationAndAttribution(t *testing.T) {
	q := Query{Level: "campaign", Preset: "last_7d", Fields: "spend", Attribution: "adset"}
	path, v, err := q.Values("act_123")
	if err != nil || path != "act_123/insights" || v.Get("use_unified_attribution_setting") != "true" {
		t.Fatal(path, v, err)
	}
	q.Windows = "7d_click"
	if _, _, err = q.Values("act_123"); err == nil {
		t.Fatal("conflicting attribution")
	}
	q.Attribution = "windows"
	_, v, err = q.Values("act_123")
	if err != nil || v.Get("use_unified_attribution_setting") != "" {
		t.Fatal(v, err)
	}
	q.Preset = ""
	q.Since = "2026-10-09"
	q.Until = "2026-10-01"
	if _, _, err = q.Values("act_123"); err == nil {
		t.Fatal("reversed dates")
	}
	q.Preset = "last_7d"
	if _, _, err = q.Values("act_123"); err == nil {
		t.Fatal("conflicting dates")
	}
}
func TestQueryScope(t *testing.T) {
	q := Query{Level: "account", Campaign: "456", Preset: "yesterday"}
	if _, _, err := q.Values("act_123"); err == nil {
		t.Fatal("ancestor level")
	}
	q.Level = "ad"
	if _, _, err := q.Values("act_123"); err != nil {
		t.Fatal(err)
	}
}
