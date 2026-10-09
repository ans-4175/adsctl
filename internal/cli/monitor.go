package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	"adsctl/internal/document"
	"adsctl/internal/monitoring"
	"adsctl/internal/output"
	"adsctl/internal/platform/meta/graph"
	"adsctl/internal/platform/meta/insights"
	"adsctl/internal/reporting"
	"adsctl/internal/state"
	"github.com/spf13/cobra"
)

func (a *App) addMonitor(meta *cobra.Command) {
	g := group("monitor", "Evaluate one-shot read-only rules")
	meta.AddCommand(g)
	for _, op := range []string{"validate", "check"} {
		var file, dir, portfolio string
		var fail bool
		effect := "local"
		if op == "check" {
			effect = "read"
		}
		cmd := action(op, "Validate or evaluate a versioned rule document", effect, func(c *cobra.Command, _ []string) error {
			b, err := a.readFile(file)
			if err != nil {
				return err
			}
			var doc monitoring.Document
			if err = document.Decode(b, &doc); err != nil {
				return err
			}
			if err = doc.Validate(); err != nil {
				return err
			}
			if op == "validate" {
				return a.emit(c, map[string]any{"valid": true, "document": doc})
			}
			dir, err = state.Dir(dir)
			if err != nil {
				return err
			}
			if portfolio != "" {
				if c.Flags().Changed("profile") || c.Flags().Changed("account") {
					return output.Fail("USAGE", "Portfolio cannot be combined with explicit profile/account.", 2)
				}
				cfg, _, err := a.loadConfig()
				if err != nil {
					return err
				}
				members, ok := cfg.Portfolios[portfolio]
				if !ok {
					return output.Fail("CONFIG", "Unknown portfolio.", 2)
				}
				results := []any{}
				failed, fired := false, false
				seen := map[string]bool{}
				for _, m := range members {
					if seen[m.Account] {
						return output.Fail("CONFIG", "Duplicate portfolio account.", 2)
					}
					seen[m.Account] = true
					child := *a
					child.opts.Profile = m.Profile
					child.opts.Account = m.Account
					data, err := child.checkMonitor(c, doc, dir)
					env := output.New(data, child.meta)
					if err != nil {
						env.OK = false
						env.Error = output.AsError(err)
						failed = true
					}
					if data != nil && data["fired"] == true {
						fired = true
					}
					results = append(results, env)
				}
				a.meta = map[string]any{"platform": "meta", "portfolio": portfolio, "complete": !failed}
				if failed {
					return a.partial(c, results, output.Fail("PARTIAL", "Some portfolio evaluations failed.", 9))
				}
				if fail && fired {
					return a.partial(c, results, output.Fail("FINDINGS", "Monitoring rules fired.", 11))
				}
				return a.emit(c, results)
			}
			data, err := a.checkMonitor(c, doc, dir)
			if err != nil {
				return a.partial(c, data, err)
			}
			if fail && data["fired"] == true {
				return a.partial(c, data, output.Fail("FINDINGS", "Monitoring rules fired.", 11))
			}
			return a.emit(c, data)
		})
		cmd.Flags().StringVar(&file, "file", "", "Rule document path or -")
		if op == "check" {
			cmd.Flags().StringVar(&dir, "state-dir", "", "Persistent deduplication directory")
			cmd.Flags().StringVar(&portfolio, "portfolio", "", "Named portfolio")
			cmd.Flags().BoolVar(&fail, "fail-on-finding", false, "Exit 11 when rules fire")
		}
		g.AddCommand(cmd)
	}
}
func (a *App) checkMonitor(c *cobra.Command, doc monitoring.Document, dir string) (map[string]any, error) {
	r, err := a.runtime(c, true, false)
	if err != nil {
		return nil, err
	}
	defer r.Cancel()
	raw, _ := json.Marshal(doc)
	hash := sha256.Sum256(append(raw, []byte(r.Profile+"|"+r.Account+"|"+r.Version)...))
	key := "monitor_" + hex.EncodeToString(hash[:16])
	unlock, err := state.Lock(dir, key)
	if err != nil {
		return nil, err
	}
	defer unlock()
	previous := map[string]bool{}
	path, _ := state.Path(dir, key)
	if _, err = os.Stat(path); err == nil {
		if err = state.Read(path, &previous); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	rows := []any{}
	queries := []any{}
	unavailable := false
	fired := false
	for _, rule := range doc.Rules {
		var result graph.ListResult
		if rule.Type == "delivery-status" {
			result, err = r.Client.List(r.Ctx, r.Account+"/"+rule.Resource, url.Values{"fields": {"id,name,effective_status,status"}}, graph.PageOptions{All: true, PageSize: 100, MaxItems: 10000})
		} else {
			account, e := r.Client.Request(r.Ctx, "GET", r.Account, url.Values{"fields": {"id,currency,timezone_name"}})
			if e != nil {
				return map[string]any{"results": rows}, e
			}
			if rule.Currency != "" && account.Value["currency"] != rule.Currency {
				return map[string]any{"results": rows}, output.Fail("CURRENCY_MISMATCH", "Rule currency differs from account currency.", 5)
			}
			zone, _ := account.Value["timezone_name"].(string)
			start, end, e := reporting.Period(rule.Preset, "", "", zone, time.Now())
			if e != nil {
				return nil, e
			}
			start = start.AddDate(0, 0, -rule.ExcludeRecent)
			end = end.AddDate(0, 0, -rule.ExcludeRecent)
			loc, zoneErr := time.LoadLocation(zone)
			if zoneErr != nil {
				return nil, zoneErr
			}
			if end.Format("2006-01-02") >= time.Now().In(loc).Format("2006-01-02") || end.Before(start) {
				return nil, output.Fail("VALIDATION", "Rules must use completed account-local days.", 5)
			}
			fields := joinFields("spend,actions,action_values", rule.Metric)
			if rule.Metric == "cpa" || rule.Metric == "roas" {
				fields = "spend,actions,action_values"
			}
			query := insights.Query{Level: rule.Resource, Since: start.Format("2006-01-02"), Until: end.Format("2006-01-02"), Increment: "all_days", Fields: fields, Attribution: rule.Attribution, ReportTime: rule.ReportTime}
			edge, values, e := query.Values(r.Account)
			if e != nil {
				return nil, e
			}
			queries = append(queries, map[string]any{"rule_id": rule.ID, "query": values, "timezone": zone, "currency": account.Value["currency"]})
			result, err = r.Client.List(r.Ctx, edge, values, graph.PageOptions{All: true, PageSize: 100, MaxItems: 10000})
		}
		if err != nil {
			return map[string]any{"results": rows, "queries": queries}, err
		}
		if !result.Complete {
			return map[string]any{"results": rows}, output.Fail("INCOMPLETE", "Rule input exceeded row bound; state unchanged.", 9)
		}
		if len(result.Rows) == 0 && rule.Type == "metric-threshold" {
			rows = append(rows, map[string]any{"rule_id": rule.ID, "entity_id": r.Account, "state": "unavailable", "reason": "no_rows"})
			unavailable = true
		}
		for _, v := range result.Rows {
			m, ok := v.(map[string]any)
			if !ok {
				return nil, output.Fail("RESPONSE_INVALID", "Invalid report row.", 8)
			}
			evaluated := monitoring.Evaluate(rule, m)
			if evaluated["state"] == "unavailable" {
				unavailable = true
			}
			if evaluated["state"] == "firing" {
				fired = true
			}
			rows = append(rows, evaluated)
		}
	}
	data := map[string]any{"id": doc.ID, "results": rows, "queries": queries, "fired": fired, "complete": !unavailable, "state_key": key}
	if unavailable {
		return data, output.Fail("INSUFFICIENT_DATA", "Required rule evidence is unavailable; deduplication state unchanged.", 9)
	}
	// Insufficient evidence must not resolve a previous finding.
	next, events := monitoring.Transitions(rows, previous)
	for _, v := range rows {
		m := v.(map[string]any)
		if m["state"] == "insufficient_evidence" {
			key := fmt.Sprint(m["rule_id"]) + ":" + fmt.Sprint(m["entity_id"])
			if previous[key] {
				next[key] = true
			}
		}
	}
	filtered := []any{}
	for _, e := range events {
		m := e.(map[string]any)
		if m["transition"] == "recovered" && next[fmt.Sprint(m["key"])] {
			continue
		}
		filtered = append(filtered, e)
	}
	data["transitions"] = filtered
	if err = state.Save(dir, key, next); err != nil {
		return data, err
	}
	return data, nil
}
