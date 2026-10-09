package monitoring

import (
	"adsctl/internal/document"
	"testing"
)

func TestRulesStrictAndEvidence(t *testing.T) {
	input := []byte("version: 1\nid: daily\nrules:\n  - id: expensive\n    type: metric-threshold\n    resource: campaign\n    date_preset: last_7d\n    metric: cpa\n    operator: gt\n    threshold: '30'\n    currency: USD\n    action_type: purchase\n    min_spend: '100'\n    min_actions: 5\n")
	var doc Document
	if err := document.Decode(input, &doc); err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(); err != nil {
		t.Fatal(err)
	}
	rule := doc.Rules[0]
	row := map[string]any{"campaign_id": "1", "spend": "200", "actions": []any{map[string]any{"action_type": "purchase", "value": "5"}}}
	if Evaluate(rule, row)["state"] != "firing" {
		t.Fatal(Evaluate(rule, row))
	}
	row["spend"] = "50"
	if Evaluate(rule, row)["state"] != "insufficient_evidence" {
		t.Fatal(Evaluate(rule, row))
	}
	delete(row, "spend")
	if Evaluate(rule, row)["state"] != "unavailable" {
		t.Fatal(Evaluate(rule, row))
	}
	var invalid Document
	if document.Decode(append(input, []byte("extra: true\n")...), &invalid) == nil {
		t.Fatal("unknown field accepted")
	}
}
func TestFindingTransitions(t *testing.T) {
	rows := []any{map[string]any{"rule_id": "a", "entity_id": "1", "state": "firing"}}
	next, events := Transitions(rows, map[string]bool{})
	if len(next) != 1 || events[0].(map[string]any)["transition"] != "started" {
		t.Fatal(next, events)
	}
	_, events = Transitions(rows, next)
	if events[0].(map[string]any)["transition"] != "ongoing" {
		t.Fatal(events)
	}
	_, events = Transitions(nil, next)
	if events[0].(map[string]any)["transition"] != "recovered" {
		t.Fatal(events)
	}
}
