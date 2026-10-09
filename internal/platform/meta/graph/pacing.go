package graph

import (
	"adsctl/internal/output"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type pacer struct {
	mu   sync.Mutex
	next time.Time
}

func (c *Client) pace(ctx context.Context) error {
	c.pacing.mu.Lock()
	delay := time.Until(c.pacing.next)
	c.pacing.mu.Unlock()
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return output.Fail("TIMEOUT", "Deadline exceeded while pacing Graph usage.", 7)
	case <-timer.C:
		return nil
	}
}
func (c *Client) observe(h http.Header) {
	delay := time.Duration(0)
	for _, key := range []string{"X-App-Usage", "X-Ad-Account-Usage", "X-Business-Use-Case-Usage", "X-Fb-Ads-Insights-Throttle", "X-Fb-Ads-Insights-Reach-Throttle"} {
		var value any
		if json.Unmarshal([]byte(h.Get(key)), &value) == nil && highUsage(value) {
			delay = c.Backoff * 4
		}
	}
	if raw := h.Get("Retry-After"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err == nil && seconds > 0 {
			delay = time.Duration(seconds) * time.Second
		} else if at, err := http.ParseTime(raw); err == nil && time.Until(at) > delay {
			delay = time.Until(at)
		}
	}
	if delay > time.Minute {
		delay = time.Minute
	}
	if delay <= 0 {
		return
	}
	c.pacing.mu.Lock()
	defer c.pacing.mu.Unlock()
	next := time.Now().Add(delay)
	if next.After(c.pacing.next) {
		c.pacing.next = next
	}
}
func highUsage(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		for key, value := range x {
			switch key {
			case "call_count", "total_time", "total_cputime", "acc_id_util_pct", "app_id_util_pct":
				if n, ok := value.(float64); ok && n >= 90 {
					return true
				}
			}
			if highUsage(value) {
				return true
			}
		}
	case []any:
		for _, value := range x {
			if highUsage(value) {
				return true
			}
		}
	}
	return false
}
