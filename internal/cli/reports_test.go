package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
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
