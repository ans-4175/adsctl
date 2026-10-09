// Package insights validates and compiles reproducible Meta report queries.
package insights

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"

	"adsctl/internal/config"
	"adsctl/internal/output"
)

type Query struct {
	Level, Preset, Since, Until, Increment, Fields, Breakdowns, ActionBreakdowns, Attribution, Windows, ReportTime, Campaign, Adset, Ad, Sort string
	Filtering                                                                                                                                 []any
}

func (q Query) Values(account string) (string, url.Values, error) {
	fail := func(msg string) (string, url.Values, error) { return "", nil, output.Fail("USAGE", msg, 2) }
	levels := map[string]int{"account": 0, "campaign": 1, "adset": 2, "ad": 3}
	root := account
	rootLevel := "account"
	count := 0
	for _, s := range []struct{ id, level string }{{q.Campaign, "campaign"}, {q.Adset, "adset"}, {q.Ad, "ad"}} {
		if s.id != "" {
			if err := config.ID(s.id); err != nil {
				return "", nil, err
			}
			root = s.id
			rootLevel = s.level
			count++
		}
	}
	if count > 1 {
		return fail("Choose only one report root: campaign, adset or ad.")
	}
	level := q.Level
	if level == "" {
		level = rootLevel
	}
	rank, ok := levels[level]
	if !ok || rank < levels[rootLevel] {
		return fail("Report level must be the root level or a descendant.")
	}
	if q.Preset != "" && (q.Since != "" || q.Until != "") {
		return fail("Date preset and explicit dates are mutually exclusive.")
	}
	if q.Preset == "" {
		start, e1 := time.Parse("2006-01-02", q.Since)
		end, e2 := time.Parse("2006-01-02", q.Until)
		if e1 != nil || e2 != nil || end.Before(start) {
			return fail("Supply a date preset or a complete ordered YYYY-MM-DD range.")
		}
	}
	inc := q.Increment
	if inc == "" {
		inc = "all_days"
	}
	if inc != "all_days" && inc != "monthly" {
		n, err := strconv.Atoi(inc)
		if err != nil || n < 1 || n > 90 {
			return fail("Time increment must be all_days, monthly or 1..90.")
		}
	}
	attr := q.Attribution
	if attr == "" {
		attr = "adset"
	}
	v := url.Values{"level": {level}, "time_increment": {inc}, "fields": {fields(q.Fields, level)}}
	switch attr {
	case "adset":
		v.Set("use_unified_attribution_setting", "true")
	case "account":
		v.Set("use_account_attribution_setting", "true")
	case "windows":
		if q.Windows == "" {
			return fail("Attribution windows are required with windows mode.")
		}
		b, _ := json.Marshal(strings.Split(q.Windows, ","))
		v.Set("action_attribution_windows", string(b))
	default:
		return fail("Attribution must be adset, account or windows.")
	}
	if attr != "windows" && q.Windows != "" {
		return fail("Explicit attribution windows cannot be combined with account/adset attribution.")
	}
	reportTime := q.ReportTime
	if reportTime == "" {
		reportTime = "impression"
	}
	if reportTime != "impression" && reportTime != "conversion" && reportTime != "mixed" {
		return fail("Action report time must be impression, conversion or mixed.")
	}
	v.Set("action_report_time", reportTime)
	if q.Preset != "" {
		v.Set("date_preset", q.Preset)
	} else {
		b, _ := json.Marshal(map[string]string{"since": q.Since, "until": q.Until})
		v.Set("time_range", string(b))
	}
	for key, s := range map[string]string{"breakdowns": q.Breakdowns, "action_breakdowns": q.ActionBreakdowns, "sort": q.Sort} {
		if s != "" {
			v.Set(key, s)
		}
	}
	if q.Filtering != nil {
		b, err := json.Marshal(q.Filtering)
		if err != nil {
			return "", nil, err
		}
		v.Set("filtering", string(b))
	}
	return root + "/insights", v, nil
}

func fields(requested, level string) string {
	seen := map[string]bool{}
	out := []string{}
	for _, p := range []string{requested, "date_start,date_stop," + level + "_id," + level + "_name"} {
		for _, f := range strings.Split(p, ",") {
			if f != "" && !seen[f] {
				out = append(out, f)
				seen[f] = true
			}
		}
	}
	return strings.Join(out, ",")
}
