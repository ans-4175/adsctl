package reporting

import (
	"testing"
	"time"
)

func TestDerivedMetricRequiresExplicitEvent(t *testing.T) {
	row := map[string]any{"spend": "30.00", "actions": []any{map[string]any{"action_type": "purchase", "value": "3"}, map[string]any{"action_type": "other_purchase", "value": "10"}}}
	v, ok := Metric(row, "cpa", "purchase")
	if !ok || Decimal(v) != "10.000000" {
		t.Fatal(v, ok)
	}
	if _, ok = Metric(row, "cpa", ""); ok {
		t.Fatal("implicit action")
	}
	if _, ok = Metric(row, "cpa", "missing"); ok {
		t.Fatal("missing treated as zero")
	}
	a, _ := Number("1.01")
	b, _ := Number("0")
	if Change(a, b)["percent_change"] != nil {
		t.Fatal("zero baseline percent")
	}
}
func TestPeriodUsesAccountZoneAndCalendarDays(t *testing.T) {
	now := time.Date(2026, 10, 9, 0, 30, 0, 0, time.UTC)
	start, end, err := Period("last_7d", "", "", "America/Los_Angeles", now)
	if err != nil || start.Format("2006-01-02") != "2026-10-01" || end.Format("2006-01-02") != "2026-10-07" {
		t.Fatal(start, end, err)
	}
}
