// Package monitoring validates and evaluates deterministic, read-only rules.
package monitoring

import (
	"adsctl/internal/output"
	"adsctl/internal/reporting"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

type Document struct {
	Version int    `yaml:"version" json:"version"`
	ID      string `yaml:"id" json:"id"`
	Rules   []Rule `yaml:"rules" json:"rules"`
}
type Rule struct {
	ID            string   `yaml:"id" json:"id"`
	Type          string   `yaml:"type" json:"type"`
	Resource      string   `yaml:"resource" json:"resource"`
	Statuses      []string `yaml:"effective_status_in" json:"effective_status_in,omitempty"`
	Preset        string   `yaml:"date_preset" json:"date_preset,omitempty"`
	Attribution   string   `yaml:"attribution" json:"attribution,omitempty"`
	ReportTime    string   `yaml:"action_report_time" json:"action_report_time,omitempty"`
	Metric        string   `yaml:"metric" json:"metric,omitempty"`
	Operator      string   `yaml:"operator" json:"operator,omitempty"`
	Threshold     string   `yaml:"threshold" json:"threshold,omitempty"`
	Currency      string   `yaml:"currency" json:"currency,omitempty"`
	Action        string   `yaml:"action_type" json:"action_type,omitempty"`
	MinSpend      string   `yaml:"min_spend" json:"min_spend,omitempty"`
	MinActions    int      `yaml:"min_actions" json:"min_actions,omitempty"`
	ExcludeRecent int      `yaml:"exclude_recent_days" json:"exclude_recent_days,omitempty"`
}

var name = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
var metric = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func (d *Document) Validate() error {
	if d.Version != 1 || !name.MatchString(d.ID) || len(d.Rules) == 0 || len(d.Rules) > 100 {
		return output.Fail("VALIDATION", "Rules require version 1, valid id and 1..100 rules.", 5)
	}
	seen := map[string]bool{}
	for i := range d.Rules {
		r := &d.Rules[i]
		if !name.MatchString(r.ID) || seen[r.ID] {
			return output.Fail("VALIDATION", "Rule IDs must be valid and unique.", 5)
		}
		seen[r.ID] = true
		if r.Type == "delivery-status" {
			if (r.Resource != "ads" && r.Resource != "adsets" && r.Resource != "campaigns") || len(r.Statuses) == 0 {
				return output.Fail("VALIDATION", "Delivery rules require campaigns/adsets/ads and effective_status_in.", 5)
			}
			continue
		}
		if r.Type != "metric-threshold" || !strings.Contains("|account|campaign|adset|ad|", "|"+r.Resource+"|") || !metric.MatchString(r.Metric) || r.Preset == "" {
			return output.Fail("VALIDATION", "Invalid metric rule type, resource, metric or date preset.", 5)
		}
		n, ok := reporting.Number(r.Threshold)
		if !ok || n.Sign() < 0 {
			return output.Fail("VALIDATION", "Threshold requires a nonnegative decimal.", 5)
		}
		if r.MinSpend == "" {
			r.MinSpend = "0"
		}
		n, ok = reporting.Number(r.MinSpend)
		if !ok || n.Sign() < 0 || r.MinActions < 0 || r.ExcludeRecent < 0 || r.ExcludeRecent > 366 {
			return output.Fail("VALIDATION", "Evidence bounds must be nonnegative.", 5)
		}
		if r.Operator != "gt" && r.Operator != "gte" && r.Operator != "lt" && r.Operator != "lte" {
			return output.Fail("VALIDATION", "Operator must be gt, gte, lt or lte.", 5)
		}
		if (r.Metric == "cpa" || r.Metric == "roas" || r.MinActions > 0) && r.Action == "" {
			return output.Fail("VALIDATION", "Derived metrics and min_actions require action_type.", 5)
		}
		if (r.Metric == "spend" || r.Metric == "cpa" || r.Metric == "cpc" || r.Metric == "cpm" || n.Sign() > 0) && r.Currency == "" {
			return output.Fail("VALIDATION", "Money thresholds/min_spend require explicit currency.", 5)
		}
		if r.Attribution == "" {
			r.Attribution = "adset"
		}
		if r.Attribution != "adset" && r.Attribution != "account" {
			return output.Fail("VALIDATION", "Rule attribution must be adset or account.", 5)
		}
		if r.ReportTime == "" {
			r.ReportTime = "impression"
		}
		if r.ReportTime != "impression" && r.ReportTime != "conversion" && r.ReportTime != "mixed" {
			return output.Fail("VALIDATION", "Invalid action_report_time.", 5)
		}
	}
	return nil
}
func Evaluate(r Rule, row map[string]any) map[string]any {
	id := fmt.Sprint(row[r.Resource+"_id"])
	if r.Type == "delivery-status" {
		id = fmt.Sprint(row["id"])
	}
	out := map[string]any{"rule_id": r.ID, "entity_id": id, "state": "clear", "data": row}
	if r.Type == "delivery-status" {
		status, ok := row["effective_status"].(string)
		if !ok {
			out["state"] = "unavailable"
			return out
		}
		for _, s := range r.Statuses {
			if status == s {
				out["state"] = "firing"
			}
		}
		return out
	}
	spend, ok := reporting.Number(row["spend"])
	minSpend, _ := reporting.Number(r.MinSpend)
	if !ok || minSpend == nil {
		out["state"] = "unavailable"
		return out
	}
	if spend.Cmp(minSpend) < 0 {
		out["state"] = "insufficient_evidence"
		return out
	}
	if r.MinActions > 0 {
		actions, ok := reporting.Action(row, "actions", r.Action)
		if !ok || actions.Cmp(big.NewRat(int64(r.MinActions), 1)) < 0 {
			out["state"] = "insufficient_evidence"
			return out
		}
	}
	value, ok := reporting.Metric(row, r.Metric, r.Action)
	if !ok {
		out["state"] = "unavailable"
		return out
	}
	limit, _ := reporting.Number(r.Threshold)
	cmp := value.Cmp(limit)
	fire := r.Operator == "gt" && cmp > 0 || r.Operator == "gte" && cmp >= 0 || r.Operator == "lt" && cmp < 0 || r.Operator == "lte" && cmp <= 0
	if fire {
		out["state"] = "firing"
	}
	out["value"] = reporting.Decimal(value)
	out["threshold"] = r.Threshold
	return out
}
func Transitions(rows []any, previous map[string]bool) (map[string]bool, []any) {
	next := map[string]bool{}
	events := []any{}
	for _, v := range rows {
		m := v.(map[string]any)
		key := fmt.Sprint(m["rule_id"]) + ":" + fmt.Sprint(m["entity_id"])
		if m["state"] == "firing" {
			next[key] = true
			event := "started"
			if previous[key] {
				event = "ongoing"
			}
			m["transition"] = event
			events = append(events, map[string]any{"key": key, "transition": event})
		}
	}
	for key := range previous {
		if !next[key] {
			events = append(events, map[string]any{"key": key, "transition": "recovered"})
		}
	}
	return next, events
}
