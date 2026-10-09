package cli

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"adsctl/internal/config"
	"adsctl/internal/output"
	"adsctl/internal/reporting"
	"github.com/spf13/cobra"
)

type reportFlags struct {
	Metric, Order, Action, MinSpend, Compare, CompareSince, CompareUntil, Format, Directory, Portfolio string
	Top, Concurrency                                                                                   int
	IncludePartial                                                                                     bool
}

func (a *App) addReports(meta *cobra.Command) {
	g := group("reports", "Summarize, compare, rank and export performance")
	meta.AddCommand(g)
	for _, op := range []string{"summary", "compare", "rank", "breakdown", "export"} {
		cmd := action(op, "Generate a complete "+op+" report", "read", nil)
		q := addQueryFlags(cmd, true)
		q.Query.Level = "account"
		cmd.Flags().Lookup("level").DefValue = "account"
		q.Page.All = true
		f := &reportFlags{}
		flags := cmd.Flags()
		flags.StringVar(&f.Portfolio, "portfolio", "", "Named read-only account portfolio")
		flags.IntVar(&f.Concurrency, "concurrency", 2, "Maximum concurrent portfolio accounts, 1..16")
		flags.BoolVar(&f.IncludePartial, "include-partial-period", false, "Allow periods including the current account-local day")
		if op == "rank" {
			flags.StringVar(&f.Metric, "metric", "spend", "Native metric or cpa/roas")
			flags.StringVar(&f.Order, "order", "desc", "asc or desc")
			flags.IntVar(&f.Top, "top", 10, "Top result count")
			flags.StringVar(&f.Action, "action-type", "", "Explicit event for cpa/roas")
			flags.StringVar(&f.MinSpend, "min-spend", "0", "Minimum spend in account currency major units")
		}
		if op == "compare" {
			flags.StringVar(&f.Compare, "compare", "previous-period", "previous-period or explicit")
			flags.StringVar(&f.CompareSince, "compare-since", "", "Comparison start date")
			flags.StringVar(&f.CompareUntil, "compare-until", "", "Comparison end date")
		}
		if op == "export" {
			flags.StringVar(&f.Format, "format", "jsonl", "jsonl or csv")
			flags.StringVar(&f.Directory, "directory", "", "New export directory")
		}
		cmd.RunE = func(c *cobra.Command, _ []string) error {
			if f.Portfolio != "" {
				return a.portfolioReport(c, op, q, f)
			}
			r, err := a.runtime(c, true, false)
			if err != nil {
				return err
			}
			defer r.Cancel()
			data, err := a.report(c, r, op, q, f)
			if err != nil {
				return a.partial(c, data, err)
			}
			return a.emit(c, data)
		}
		g.AddCommand(cmd)
	}
}
func (a *App) partial(c *cobra.Command, data any, err error) error {
	if data != nil {
		env := output.New(data, a.meta)
		env.OK = false
		env.Error = output.AsError(err)
		if output.AsError(err).Exit == 11 {
			env.Meta["complete"] = true
		} else {
			env.Meta["partial"] = true
			env.Meta["complete"] = false
		}
		env.Meta["command"] = c.CommandPath()
		_ = output.Write(a.Out, a.opts.Output, env)
	}
	return err
}
func (a *App) report(c *cobra.Command, r *runtime, op string, q *queryFlags, f *reportFlags) (any, error) {
	local := *q
	local.Query = q.Query
	local.Page.All = true
	local.Page.Cursor = ""
	if op != "export" {
		local.Query.Increment = "all_days"
	}
	if op == "compare" && (local.Query.Breakdowns != "" || local.Query.ActionBreakdowns != "") {
		return nil, output.Fail("USAGE", "Comparison requires entity totals without breakdowns.", 2)
	}
	if op == "summary" && local.Query.Breakdowns != "" {
		return nil, output.Fail("USAGE", "Summary requires account totals without breakdowns; use reports breakdown.", 2)
	}
	if op == "summary" {
		local.Query.Level = "account"
	}
	if op == "rank" {
		if f.Top < 1 || (f.Order != "asc" && f.Order != "desc") {
			return nil, output.Fail("USAGE", "Require positive --top and asc/desc order.", 2)
		}
		if (f.Metric == "cpa" || f.Metric == "roas") && f.Action == "" {
			return nil, output.Fail("USAGE", "Derived cpa/roas requires --action-type.", 2)
		}
		if local.Query.Breakdowns != "" {
			return nil, output.Fail("USAGE", "Ranking requires entity totals without breakdowns.", 2)
		}
		local.Query.Fields = joinFields(local.Query.Fields, "spend", local.Query.Level+"_id,"+local.Query.Level+"_name", f.Metric)
		if f.Metric == "cpa" || f.Metric == "roas" {
			fields := []string{}
			for _, field := range strings.Split(local.Query.Fields, ",") {
				if field != "cpa" && field != "roas" {
					fields = append(fields, field)
				}
			}
			local.Query.Fields = strings.Join(fields, ",")
			local.Query.Fields = joinFields(local.Query.Fields, "actions,action_values", "")
		}
	}
	if op == "breakdown" && local.Query.Breakdowns == "" {
		return nil, output.Fail("USAGE", "Breakdown report requires --breakdowns.", 2)
	}
	path, v, err := a.prepareQuery(c, r, &local)
	if err != nil {
		return nil, err
	}
	zone, ok := a.meta["timezone"].(string)
	if !ok {
		return nil, output.Fail("TIMEZONE_UNAVAILABLE", "Account did not return a timezone.", 8)
	}
	start, end, err := reporting.Period(local.Query.Preset, local.Query.Since, local.Query.Until, zone, time.Now())
	if err != nil {
		return nil, err
	}
	if end.Before(start) {
		return nil, output.Fail("INSUFFICIENT_DATA", "No completed days in reporting period.", 9)
	}
	today := time.Now().In(start.Location())
	if end.Format("2006-01-02") >= today.Format("2006-01-02") && !f.IncludePartial {
		return nil, output.Fail("USAGE", "Current-day reports require --include-partial-period.", 2)
	}
	setRange(v, start, end)
	a.meta["query"] = v
	a.meta["resolved_period"] = map[string]string{"since": start.Format("2006-01-02"), "until": end.Format("2006-01-02")}
	local.Page.Scope = r.Profile + "|" + r.Account
	result, err := r.Client.List(r.Ctx, path, v, local.Page)
	a.meta["complete"] = result.Complete
	if err != nil {
		return result.Rows, err
	}
	if !result.Complete {
		return result.Rows, output.Fail("INCOMPLETE", "Report input exceeded its row bound.", 9)
	}
	switch op {
	case "summary":
		contributorsQuery := url.Values{}
		for key, values := range v {
			contributorsQuery[key] = append([]string(nil), values...)
		}
		contributorsQuery.Set("level", "campaign")
		fields := []string{"campaign_id", "campaign_name"}
		for _, field := range strings.Split(local.Query.Fields, ",") {
			if field != "account_id" && field != "account_name" {
				fields = append(fields, field)
			}
		}
		contributorsQuery.Set("fields", joinFields(strings.Join(fields, ",")))
		contributors, contributorErr := r.Client.List(r.Ctx, r.Account+"/insights", contributorsQuery, local.Page)
		a.meta["contributors_query"] = contributorsQuery
		data := map[string]any{"account_metrics": result.Rows, "campaign_contributors": contributors.Rows, "availability": "Missing fields are unavailable, not zero; account totals are not summed from contributors."}
		if contributorErr != nil {
			a.meta["complete"] = false
			return data, contributorErr
		}
		if !contributors.Complete {
			a.meta["complete"] = false
			return data, output.Fail("INCOMPLETE", "Contributor input exceeded row bound.", 9)
		}
		return data, nil
	case "compare":
		var previousStart, previousEnd time.Time
		if f.CompareSince != "" || f.CompareUntil != "" {
			previousStart, previousEnd, err = reporting.Period("", f.CompareSince, f.CompareUntil, zone, time.Now())
			if err != nil {
				return nil, err
			}
		} else {
			if f.Compare != "previous-period" {
				return nil, output.Fail("USAGE", "Use previous-period or explicit comparison dates.", 2)
			}
			days := 0
			for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
				days++
			}
			previousEnd = start.AddDate(0, 0, -1)
			previousStart = start.AddDate(0, 0, -days)
		}
		oldQ := url.Values{}
		for key, values := range v {
			oldQ[key] = append([]string(nil), values...)
		}
		setRange(oldQ, previousStart, previousEnd)
		old, err := r.Client.List(r.Ctx, path, oldQ, local.Page)
		if err != nil || !old.Complete {
			if err == nil {
				err = output.Fail("INCOMPLETE", "Comparison input exceeded row bound.", 9)
			}
			return nil, err
		}
		a.meta["comparison_query"] = oldQ
		return compareRows(result.Rows, old.Rows, local.Query.Level), nil
	case "rank":
		a.meta["metric"] = map[string]any{"name": f.Metric, "action_type": f.Action, "unit": map[string]string{"cpa": "account_currency_major", "roas": "ratio", "spend": "account_currency_major"}[f.Metric], "formula": map[string]string{"cpa": "spend / matching actions.value", "roas": "matching action_values.value / spend"}[f.Metric]}
		return rankRows(result.Rows, f)
	case "export":
		return a.exportRows(result.Rows, f)
	default:
		return result.Rows, nil
	}
}
func joinFields(parts ...string) string {
	seen := map[string]bool{}
	out := []string{}
	for _, p := range parts {
		for _, field := range strings.Split(p, ",") {
			if field != "" && !seen[field] {
				out = append(out, field)
				seen[field] = true
			}
		}
	}
	return strings.Join(out, ",")
}
func setRange(v url.Values, start, end time.Time) {
	v.Del("date_preset")
	b, _ := json.Marshal(map[string]string{"since": start.Format("2006-01-02"), "until": end.Format("2006-01-02")})
	v.Set("time_range", string(b))
}
func compareRows(current, previous []any, level string) map[string]any {
	key := level + "_id"
	old := map[string]map[string]any{}
	for _, row := range previous {
		if m, ok := row.(map[string]any); ok {
			old[fmt.Sprint(m[key])] = m
		}
	}
	changes := []any{}
	for _, row := range current {
		m, ok := row.(map[string]any)
		if !ok {
			continue
		}
		before := old[fmt.Sprint(m[key])]
		metrics := map[string]any{}
		for field, value := range m {
			if strings.HasSuffix(field, "_id") || strings.HasPrefix(field, "date_") {
				continue
			}
			x, ok := reporting.Number(value)
			y, ok2 := reporting.Number(before[field])
			if ok && ok2 {
				metrics[field] = reporting.Change(x, y)
			}
		}
		changes = append(changes, map[string]any{"id": m[key], "metrics": metrics, "comparison_available": before != nil})
	}
	return map[string]any{"current": current, "previous": previous, "changes": changes}
}
func rankRows(rows []any, f *reportFlags) (any, error) {
	minSpend, ok := reporting.Number(f.MinSpend)
	if !ok || minSpend.Sign() < 0 {
		return nil, output.Fail("USAGE", "min-spend must be a nonnegative decimal.", 2)
	}
	type entry struct {
		row    any
		metric *big.Rat
	}
	ranked := []entry{}
	excluded := []any{}
	for _, row := range rows {
		m, ok := row.(map[string]any)
		if !ok {
			continue
		}
		spend, ok := reporting.Number(m["spend"])
		if !ok || spend.Cmp(minSpend) < 0 {
			excluded = append(excluded, map[string]any{"data": row, "reason": "insufficient_spend"})
			continue
		}
		metric, ok := reporting.Metric(m, f.Metric, f.Action)
		if !ok {
			excluded = append(excluded, map[string]any{"data": row, "reason": "metric_unavailable"})
			continue
		}
		ranked = append(ranked, entry{row, metric})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if f.Order == "asc" {
			return ranked[i].metric.Cmp(ranked[j].metric) < 0
		}
		return ranked[i].metric.Cmp(ranked[j].metric) > 0
	})
	out := []any{}
	for _, e := range ranked[:min(f.Top, len(ranked))] {
		out = append(out, map[string]any{"data": e.row, "metric_value": reporting.Decimal(e.metric)})
	}
	return map[string]any{"metric": f.Metric, "action_type": f.Action, "ranked": out, "excluded": excluded, "eligible_count": len(ranked)}, nil
}
func (a *App) exportRows(rows []any, f *reportFlags) (any, error) {
	if f.Directory == "" || (f.Format != "csv" && f.Format != "jsonl") {
		return nil, output.Fail("USAGE", "Export requires a new --directory and csv/jsonl format.", 2)
	}
	if err := os.MkdirAll(filepath.Dir(f.Directory), 0700); err != nil {
		return nil, err
	}
	if err := os.Mkdir(f.Directory, 0700); err != nil {
		return nil, output.Fail("EXPORT_EXISTS", "Export directory exists or cannot be created.", 2)
	}
	partial := map[string]any{"complete": false, "query": a.meta}
	b, _ := json.MarshalIndent(partial, "", "  ")
	if err := config.Atomic(filepath.Join(f.Directory, "metadata.json"), b); err != nil {
		return nil, err
	}
	path := filepath.Join(f.Directory, "rows."+f.Format)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	if f.Format == "jsonl" {
		enc := json.NewEncoder(file)
		for _, row := range rows {
			if err = enc.Encode(row); err != nil {
				return nil, err
			}
		}
	} else {
		keys := map[string]bool{}
		for _, row := range rows {
			for k := range row.(map[string]any) {
				keys[k] = true
			}
		}
		cols := []string{}
		for k := range keys {
			cols = append(cols, k)
		}
		sort.Strings(cols)
		w := csv.NewWriter(file)
		if err = w.Write(cols); err != nil {
			return nil, err
		}
		for _, row := range rows {
			m := row.(map[string]any)
			cells := make([]string, len(cols))
			for i, k := range cols {
				if s, ok := m[k].(string); ok {
					cells[i] = s
				} else if m[k] != nil {
					b, _ := json.Marshal(m[k])
					cells[i] = string(b)
				}
			}
			if err = w.Write(cells); err != nil {
				return nil, err
			}
		}
		w.Flush()
		if err = w.Error(); err != nil {
			return nil, err
		}
	}
	if err = file.Sync(); err != nil {
		return nil, err
	}
	partial["complete"] = true
	b, _ = json.MarshalIndent(partial, "", "  ")
	if err = config.Atomic(filepath.Join(f.Directory, "metadata.json"), b); err != nil {
		return nil, err
	}
	return map[string]any{"directory": f.Directory, "rows": len(rows), "complete": true}, nil
}
func (a *App) portfolioReport(c *cobra.Command, op string, q *queryFlags, f *reportFlags) error {
	ctx, cancel := context.WithTimeout(c.Context(), a.opts.Timeout)
	defer cancel()
	c.SetContext(ctx)
	if c.Flags().Changed("profile") || c.Flags().Changed("account") {
		return output.Fail("USAGE", "Portfolio cannot be combined with profile/account flags.", 2)
	}
	if op == "export" {
		return output.Fail("USAGE", "Export one account at a time.", 2)
	}
	if f.Concurrency < 1 {
		return output.Fail("USAGE", "Concurrency must be positive.", 2)
	}
	cfg, _, err := a.loadConfig()
	if err != nil {
		return err
	}
	members, ok := cfg.Portfolios[f.Portfolio]
	if !ok {
		return output.Fail("CONFIG", "Unknown portfolio.", 2)
	}
	if f.Concurrency > 16 {
		return output.Fail("USAGE", "Concurrency must be 1..16.", 2)
	}
	if q.FilterFile == "-" {
		return output.Fail("USAGE", "Portfolio filters require a file path; stdin cannot be consumed by multiple accounts.", 2)
	}
	seen := map[string]bool{}
	normalized := make([]config.Member, len(members))
	for i, member := range members {
		account, e := config.Account(member.Account)
		if e != nil {
			return e
		}
		if seen[account] {
			return output.Fail("CONFIG", "Duplicate portfolio account.", 2)
		}
		seen[account] = true
		normalized[i] = config.Member{Profile: member.Profile, Account: account}
	}
	results := make([]any, len(members))
	jobs := make(chan int)
	var workers sync.WaitGroup
	diagnostics := &lockedWriter{writer: a.Err}
	for worker := 0; worker < min(f.Concurrency, len(members)); worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				member := normalized[i]
				child := *a
				child.opts.Profile = member.Profile
				child.opts.Account = member.Account
				child.meta = nil
				child.Err = diagnostics
				r, e := child.runtime(c, true, false)
				var data any
				if e == nil {
					data, e = child.report(c, r, op, q, f)
					r.Cancel()
				}
				env := output.New(data, child.meta)
				if e != nil {
					env.OK = false
					env.Error = output.AsError(e)
				}
				results[i] = env
			}
		}()
	}
	for i := range normalized {
		jobs <- i
	}
	close(jobs)
	workers.Wait()
	failed := false
	for _, v := range results {
		if !v.(output.Envelope).OK {
			failed = true
		}
	}

	a.meta = map[string]any{"platform": "meta", "portfolio": f.Portfolio, "complete": !failed}
	if failed {
		return a.partial(c, results, output.Fail("PARTIAL", "Some portfolio accounts could not be reported.", 9))
	}
	return a.emit(c, results)
}

type lockedWriter struct {
	mu     sync.Mutex
	writer io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(p)
}
