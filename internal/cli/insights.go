package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"adsctl/internal/config"
	"adsctl/internal/output"
	"adsctl/internal/platform/meta/graph"
	"adsctl/internal/platform/meta/insights"
	"github.com/spf13/cobra"
)

type queryFlags struct {
	Query      insights.Query
	FilterFile string
	Page       graph.PageOptions
}

func addQueryFlags(c *cobra.Command, defaults bool) *queryFlags {
	q := &queryFlags{}
	f := c.Flags()
	preset := ""
	if defaults {
		preset = "last_7d"
	}
	f.StringVar(&q.Query.Level, "level", "", "account, campaign, adset or ad")
	f.StringVar(&q.Query.Preset, "date-preset", preset, "Native date preset")
	f.StringVar(&q.Query.Since, "since", "", "Inclusive start date")
	f.StringVar(&q.Query.Until, "until", "", "Inclusive end date")
	f.StringVar(&q.Query.Increment, "time-increment", "all_days", "all_days, monthly or 1..90")
	f.StringVar(&q.Query.Fields, "fields", "spend,impressions,clicks", "Native report fields")
	f.StringVar(&q.Query.Breakdowns, "breakdowns", "", "Breakdown dimensions")
	f.StringVar(&q.Query.ActionBreakdowns, "action-breakdowns", "", "Action dimensions")
	f.StringVar(&q.Query.Attribution, "attribution", "adset", "adset, account or windows")
	f.StringVar(&q.Query.Windows, "attribution-windows", "", "Explicit windows")
	f.StringVar(&q.Query.ReportTime, "action-report-time", "impression", "impression, conversion or mixed")
	f.StringVar(&q.Query.Campaign, "campaign", "", "Campaign root ID")
	f.StringVar(&q.Query.Adset, "adset", "", "Ad set root ID")
	f.StringVar(&q.Query.Ad, "ad", "", "Ad root ID")
	f.StringVar(&q.Query.Sort, "sort", "", "Native sort expression")
	f.StringVar(&q.FilterFile, "filter-file", "", "Native filtering JSON array")
	f.BoolVar(&q.Page.All, "all", false, "Read all pages")
	f.StringVar(&q.Page.Cursor, "cursor", "", "Scoped continuation")
	f.IntVar(&q.Page.PageSize, "page-size", 100, "Requested page size")
	f.IntVar(&q.Page.MaxItems, "max-items", 10000, "Row bound")
	return q
}
func (a *App) prepareQuery(c *cobra.Command, r *runtime, q *queryFlags) (string, url.Values, error) {
	if c.Flags().Changed("since") || c.Flags().Changed("until") {
		if !c.Flags().Changed("date-preset") {
			q.Query.Preset = ""
		}
	}
	if q.FilterFile != "" {
		b, err := a.readFile(q.FilterFile)
		if err != nil {
			return "", nil, err
		}
		decoder := json.NewDecoder(strings.NewReader(string(b)))
		decoder.UseNumber()
		if err = decoder.Decode(&q.Query.Filtering); err != nil {
			return "", nil, output.Fail("USAGE", "Filter file must contain a JSON array.", 2)
		}
		var extra any
		if q.Query.Filtering == nil || decoder.Decode(&extra) != io.EOF {
			return "", nil, output.Fail("USAGE", "Filter file requires one JSON array.", 2)
		}
	}
	if q.Query.Level == "" && q.Query.Campaign == "" && q.Query.Adset == "" && q.Query.Ad == "" {
		return "", nil, output.Fail("USAGE", "Account report requires --level.", 2)
	}
	for resource, id := range map[string]string{"campaigns": q.Query.Campaign, "adsets": q.Query.Adset, "ads": q.Query.Ad} {
		if id != "" {
			if err := verify(r, resource, id); err != nil {
				return "", nil, err
			}
		}
	}
	path, v, err := q.Query.Values(r.Account)
	if err != nil {
		return "", nil, err
	}
	account, err := r.Client.Request(r.Ctx, "GET", r.Account, url.Values{"fields": {"id,currency,timezone_name"}})
	if err != nil {
		return "", nil, err
	}
	a.meta["currency"] = account.Value["currency"]
	a.meta["timezone"] = account.Value["timezone_name"]
	a.meta["query"] = v
	a.meta["attribution_mode"] = q.Query.Attribution
	if q.Query.Attribution == "adset" {
		a.meta["attribution_scope"] = "Per-adset settings; windows may differ across contributing ad sets."
	}
	return path, v, nil
}
func (a *App) readFile(path string) ([]byte, error) {
	var r io.Reader
	if path == "-" {
		r = a.In
	} else {
		f, err := os.Open(path)
		if err != nil {
			return nil, output.Fail("INPUT", "Cannot open input file.", 2)
		}
		defer func() { _ = f.Close() }()
		r = f
	}
	b, err := io.ReadAll(io.LimitReader(r, 8*1024*1024+1))
	if err != nil || len(b) > 8*1024*1024 {
		return nil, output.Fail("INPUT", "Input exceeds 8 MiB or cannot be read.", 2)
	}
	return b, nil
}
func (a *App) addInsights(meta *cobra.Command) {
	g := group("insights", "Query reproducible performance data")
	meta.AddCommand(g)
	query := action("query", "Fetch a synchronous Insights report", "read", nil)
	q := addQueryFlags(query, false)
	query.RunE = func(c *cobra.Command, _ []string) error {
		r, err := a.runtime(c, true, false)
		if err != nil {
			return err
		}
		defer r.Cancel()
		path, v, err := a.prepareQuery(c, r, q)
		if err != nil {
			return err
		}
		return a.list(c, r, path, v, q.Page)
	}
	g.AddCommand(query)
	jobs := group("jobs", "Submit and resume asynchronous reports")
	g.AddCommand(jobs)
	create := action("create", "Submit an asynchronous report (no advertising changes)", "report-job", nil)
	jq := addQueryFlags(create, false)
	var wait bool
	create.Flags().BoolVar(&wait, "wait", false, "Wait for job completion")
	create.RunE = func(c *cobra.Command, _ []string) error {
		r, err := a.runtime(c, true, false)
		if err != nil {
			return err
		}
		defer r.Cancel()
		path, v, err := a.prepareQuery(c, r, jq)
		if err != nil {
			return err
		}
		resp, err := r.Client.Request(r.Ctx, "POST", path, v)
		if err != nil {
			return err
		}
		id := fmt.Sprint(resp.Value["report_run_id"])
		if err = config.ID(id); err != nil {
			return output.Fail("RESPONSE_INVALID", "Report submission returned no valid report_run_id.", 8)
		}
		a.meta["report_run_id"] = id
		if !wait {
			return a.emit(c, resp.Value)
		}
		value, err := pollJob(r, id)
		if err != nil {
			return err
		}
		return a.emit(c, value)
	}
	jobs.AddCommand(create)
	for _, op := range []string{"get", "wait", "results"} {
		var all bool
		var cursor string
		var pageSize, maxItems int
		cmd := action(op+" ID", "Read asynchronous report "+op, "read", func(c *cobra.Command, args []string) error {
			r, err := a.runtime(c, true, false)
			if err != nil {
				return err
			}
			defer r.Cancel()
			id := args[0]
			if err = config.ID(id); err != nil {
				return err
			}
			a.meta["report_run_id"] = id
			value, err := getJob(r, id)
			if err != nil {
				return err
			}
			if op == "wait" {
				value, err = pollJob(r, id)
				if err != nil {
					return err
				}
			}
			if op != "results" {
				return a.emit(c, value)
			}
			if value["async_status"] != "Job Completed" {
				return output.Fail("JOB_PENDING", "Report job has not completed.", 8)
			}
			return a.list(c, r, id+"/insights", nil, graph.PageOptions{Scope: r.Profile + "|" + r.Account, All: all, Cursor: cursor, PageSize: pageSize, MaxItems: maxItems})
		})
		cmd.Args = cobra.ExactArgs(1)
		if op == "results" {
			cmd.Flags().BoolVar(&all, "all", false, "Read all pages")
			cmd.Flags().StringVar(&cursor, "cursor", "", "Continuation")
			cmd.Flags().IntVar(&pageSize, "page-size", 100, "Page size")
			cmd.Flags().IntVar(&maxItems, "max-items", 10000, "Row bound")
		}
		jobs.AddCommand(cmd)
	}
}
func getJob(r *runtime, id string) (map[string]any, error) {
	v, err := r.Client.Request(r.Ctx, "GET", id, url.Values{"fields": {"id,account_id,async_status,async_percent_completion,error_code,error_message,error_subcode"}})
	if err != nil {
		return nil, err
	}
	if strings.TrimPrefix(fmt.Sprint(v.Value["account_id"]), "act_") != strings.TrimPrefix(r.Account, "act_") {
		return nil, output.Fail("ACCOUNT_MISMATCH", "Report belongs to another account.", 4)
	}
	return v.Value, nil
}
func pollJob(r *runtime, id string) (map[string]any, error) {
	delay := time.Second
	for {
		v, err := getJob(r, id)
		if err != nil {
			return nil, err
		}
		status := fmt.Sprint(v["async_status"])
		if status == "Job Completed" {
			return v, nil
		}
		if strings.Contains(status, "Failed") || strings.Contains(status, "Skipped") {
			e := output.Fail("JOB_FAILED", "Meta report job failed.", 8)
			e.Provider = v
			return nil, e
		}
		timer := time.NewTimer(delay)
		delay = min(delay*2, 10*time.Second)
		select {
		case <-r.Ctx.Done():
			timer.Stop()
			return nil, output.Fail("TIMEOUT", "Report pending; resume using its report_run_id.", 7)
		case <-timer.C:
		}
	}
}
