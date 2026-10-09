package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func fixtureClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	c := New("fixture-secret", "v25.0")
	c.BaseURL = s.URL
	c.Backoff = 0
	return c
}
func TestLosslessAuthAndSafePagination(t *testing.T) {
	calls := 0
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer fixture-secret" || r.URL.Query().Get("access_token") != "" {
			t.Error("unsafe auth")
		}
		if r.URL.Query().Get("after") == "" {
			_, _ = fmt.Fprint(w, `{"data":[{"id":"9007199254740993","spend":"1.01"}],"paging":{"next":"https://untrusted.invalid/?access_token=leak&after=second","cursors":{"after":"second"}}}`)
		} else {
			_, _ = fmt.Fprint(w, `{"data":[{"id":9007199254740994}]}`)
		}
	})
	first, err := c.List(context.Background(), "act_123/campaigns", url.Values{}, PageOptions{Scope: "profile", PageSize: 1, MaxItems: 10})
	if err != nil || first.Complete || first.Cursor == "" {
		t.Fatal(first, err)
	}
	if strings.Contains(first.Cursor, "fixture-secret") {
		t.Fatal("secret cursor")
	}
	second, err := c.List(context.Background(), "act_123/campaigns", url.Values{}, PageOptions{Scope: "profile", Cursor: first.Cursor, PageSize: 1, MaxItems: 10})
	if err != nil || !second.Complete || calls != 2 {
		t.Fatal(second, err, calls)
	}
	if second.Rows[0].(map[string]any)["id"].(json.Number).String() != "9007199254740994" {
		t.Fatal(second)
	}
	_, err = c.List(context.Background(), "act_456/campaigns", url.Values{}, PageOptions{Scope: "profile", Cursor: first.Cursor, PageSize: 1, MaxItems: 10})
	if err == nil {
		t.Fatal("cross-account cursor accepted")
	}
}
func TestRetriesReadsButNotWritesAndRedactsProvider(t *testing.T) {
	calls := 0
	c := fixtureClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = fmt.Fprint(w, `{"error":{"code":4,"message":"fixture-secret exhausted","is_transient":true}}`)
	})
	_, err := c.Request(context.Background(), "GET", "me", nil)
	if err == nil || calls != 3 || strings.Contains(err.Error(), "fixture-secret") {
		t.Fatal(err, calls)
	}
	calls = 0
	_, err = c.Request(context.Background(), "POST", "act_123/campaigns", nil)
	if err == nil || calls != 1 {
		t.Fatal(err, calls)
	}
}
func TestMalformedAndProviderFailure(t *testing.T) {
	for _, body := range []string{`[]`, `not-json`, `{"error":{"code":190,"message":"expired"}}`} {
		c := fixtureClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, body) })
		if _, err := c.Request(context.Background(), "GET", "me", nil); err == nil {
			t.Fatal(body)
		}
	}
}
func TestOverfullPageContinuationDoesNotLoseRows(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"data":[{"id":"1"},{"id":"2"},{"id":"3"}]}`)
	})
	one, err := c.List(context.Background(), "me", nil, PageOptions{Scope: "p", All: true, PageSize: 1, MaxItems: 1})
	if err != nil || one.Complete || len(one.Rows) != 1 {
		t.Fatal(one, err)
	}
	two, err := c.List(context.Background(), "me", nil, PageOptions{Scope: "p", Cursor: one.Cursor, PageSize: 1, MaxItems: 10})
	if err != nil || !two.Complete || len(two.Rows) != 2 {
		t.Fatal(two, err)
	}
}
