package cli

import (
	"adsctl/internal/config"
	"adsctl/internal/state"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRankReportResolvesDatesAndSelectsAction(t *testing.T) {
	a, out, _, path := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/insights") {
			if r.URL.Query().Get("date_preset") != "" || r.URL.Query().Get("time_range") == "" {
				t.Error("report dates unresolved")
			}
			_, _ = fmt.Fprint(w, `{"data":[{"ad_id":"1","spend":"30","actions":[{"action_type":"purchase","value":"3"}]},{"ad_id":"2","spend":"40","actions":[{"action_type":"purchase","value":"2"}]}]}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"id":"act_123","currency":"USD","timezone_name":"Asia/Jakarta"}`)
	})
	err := executeTest(a, []string{"--config", path, "meta", "reports", "rank", "--profile", "test", "--account", "123", "--level", "ad", "--metric", "cpa", "--action-type", "purchase", "--order", "asc", "--top", "1"})
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Data struct {
			Ranked []struct {
				Data   map[string]any
				Metric string `json:"metric_value"`
			}
		}
	}
	if err = json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Data.Ranked) != 1 || env.Data.Ranked[0].Data["ad_id"] != "1" || env.Data.Ranked[0].Metric != "10.000000" {
		t.Fatal(out.String())
	}
}
func TestJobResultsRejectCrossAccount(t *testing.T) {
	a, _, _, path := testApp(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"id":"303","account_id":"999","async_status":"Job Completed"}`)
	})
	if err := executeTest(a, []string{"--config", path, "meta", "insights", "jobs", "results", "303", "--profile", "test", "--account", "123"}); err == nil {
		t.Fatal("cross-account report accepted")
	}
}
func TestCompleteReportCannotRankTruncatedRows(t *testing.T) {
	a, out, _, path := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/insights") {
			_, _ = fmt.Fprint(w, `{"data":[{"ad_id":"1","spend":"10"},{"ad_id":"2","spend":"20"}]}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"currency":"USD","timezone_name":"UTC"}`)
	})
	err := executeTest(a, []string{"--config", path, "meta", "reports", "rank", "--profile", "test", "--account", "123", "--level", "ad", "--max-items", "1"})
	if err == nil || !strings.Contains(out.String(), `"ok": false`) {
		t.Fatal(err, out.String())
	}
}

func TestPortfolioReportsRunBoundedWorkersWithoutCurrencyAggregation(t *testing.T) {
	var arrivals atomic.Int32
	ready := make(chan struct{})
	a, out, _, path := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/insights") {
			if arrivals.Add(1) == 2 {
				close(ready)
			}
			select {
			case <-ready:
			case <-time.After(3 * time.Second):
				t.Error("portfolio workers did not run concurrently")
			}
			currency := "USD"
			if strings.Contains(r.URL.Path, "456") {
				currency = "IDR"
			}
			_, _ = fmt.Fprintf(w, `{"currency":%q,"timezone_name":"UTC"}`, currency)
			return
		}
		if r.URL.Query().Get("level") == "campaign" {
			_, _ = fmt.Fprint(w, `{"data":[{"campaign_id":"1","spend":"10"}]}`)
		} else {
			_, _ = fmt.Fprint(w, `{"data":[{"account_id":"123","spend":"10"}]}`)
		}
	})
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Portfolios = map[string][]config.Member{"clients": {{Profile: "test", Account: "act_123"}, {Profile: "test", Account: "act_456"}}}
	if err = cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	if err = executeTest(a, []string{"--config", path, "meta", "reports", "summary", "--portfolio", "clients", "--concurrency", "2"}); err != nil {
		t.Fatal(err, out.String())
	}
	if !strings.Contains(out.String(), `"currency": "USD"`) || !strings.Contains(out.String(), `"currency": "IDR"`) || !strings.Contains(out.String(), `"campaign_contributors"`) {
		t.Fatal(out.String())
	}
}

func TestExportPreservesNestedNativeFieldsAndRejectsReplacement(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "export")
	a := &App{meta: map[string]any{"account_id": "act_123", "currency": "USD"}}
	rows := []any{map[string]any{"ad_id": "9007199254740993", "spend": "1.01", "actions": []any{map[string]any{"action_type": "purchase", "value": "2"}}}}
	receipt, err := a.exportRows(rows, &reportFlags{Directory: directory, Format: "csv"})
	if err != nil || receipt == nil {
		t.Fatal(err, receipt)
	}
	data, err := os.ReadFile(filepath.Join(directory, "rows.csv"))
	if err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(strings.NewReader(string(data))).ReadAll()
	if err != nil || len(records) != 2 {
		t.Fatal(err, records)
	}
	var actions []map[string]string
	if err = json.Unmarshal([]byte(records[1][0]), &actions); err != nil || actions[0]["action_type"] != "purchase" {
		t.Fatal(err, records)
	}
	if records[1][1] != "9007199254740993" {
		t.Fatal(records)
	}
	var metadata map[string]any
	if err = state.Read(filepath.Join(directory, "metadata.json"), &metadata); err != nil || metadata["complete"] != true {
		t.Fatal(err, metadata)
	}
	if _, err = a.exportRows(rows, &reportFlags{Directory: directory, Format: "csv"}); err == nil {
		t.Fatal("existing export replaced")
	}
}
func TestComparisonDoesNotInventPercentFromZero(t *testing.T) {
	data := compareRows([]any{map[string]any{"campaign_id": "1", "spend": "1.01", "clicks": "10"}}, []any{map[string]any{"campaign_id": "1", "spend": "0", "clicks": "5"}}, "campaign")
	change := data["changes"].([]any)[0].(map[string]any)["metrics"].(map[string]any)
	spend := change["spend"].(map[string]any)
	if spend["percent_change"] != nil || spend["reason"] != "zero_baseline" {
		t.Fatal(spend)
	}
	if change["clicks"].(map[string]any)["percent_change"] != "100.000000" {
		t.Fatal(change)
	}
}
func TestReadProfileMaySubmitAndWaitForReportingJob(t *testing.T) {
	posts := 0
	a, out, _, path := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v25.0/act_123":
			_, _ = fmt.Fprint(w, `{"currency":"USD","timezone_name":"UTC"}`)
		case "/v25.0/act_123/insights":
			posts++
			if r.Method != "POST" {
				t.Error("job submission method")
			}
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.Form.Get("use_unified_attribution_setting") != "true" {
				t.Error(r.Form)
			}
			_, _ = fmt.Fprint(w, `{"report_run_id":"303"}`)
		case "/v25.0/303":
			_, _ = fmt.Fprint(w, `{"id":"303","account_id":"123","async_status":"Job Completed","async_percent_completion":100}`)
		default:
			t.Error(r.URL.Path)
		}
	})
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	profile := cfg.Profiles["test"]
	profile.Mode = "read"
	cfg.Profiles["test"] = profile
	if err = cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	err = executeTest(a, []string{"--config", path, "meta", "insights", "jobs", "create", "--profile", "test", "--account", "123", "--level", "ad", "--date-preset", "last_7d", "--wait"})
	if err != nil || posts != 1 || !strings.Contains(out.String(), `"report_run_id": "303"`) || !strings.Contains(out.String(), `"async_status": "Job Completed"`) {
		t.Fatal(err, posts, out.String())
	}
}
