// Package reporting provides exact, explicitly defined derived report metrics.
package reporting

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"adsctl/internal/output"
)

func Number(v any) (*big.Rat, bool) {
	if v == nil {
		return nil, false
	}
	n, ok := new(big.Rat).SetString(fmt.Sprint(v))
	return n, ok
}
func Action(row map[string]any, field, event string) (*big.Rat, bool) {
	values, ok := row[field].([]any)
	if !ok {
		return nil, false
	}
	for _, v := range values {
		m, ok := v.(map[string]any)
		if ok && m["action_type"] == event {
			return Number(m["value"])
		}
	}
	return nil, false
}
func Metric(row map[string]any, name, event string) (*big.Rat, bool) {
	if name != "cpa" && name != "roas" {
		return Number(row[name])
	}
	spend, ok := Number(row["spend"])
	if !ok || event == "" {
		return nil, false
	}
	field := "actions"
	if name == "roas" {
		field = "action_values"
	}
	action, ok := Action(row, field, event)
	if !ok {
		return nil, false
	}
	if name == "cpa" {
		if action.Sign() == 0 {
			return nil, false
		}
		return new(big.Rat).Quo(spend, action), true
	}
	if spend.Sign() == 0 {
		return nil, false
	}
	return new(big.Rat).Quo(action, spend), true
}
func Decimal(n *big.Rat) string { return n.FloatString(6) }
func Change(current, previous *big.Rat) map[string]any {
	d := new(big.Rat).Sub(current, previous)
	out := map[string]any{"current": Decimal(current), "previous": Decimal(previous), "absolute_change": Decimal(d), "percent_change": nil}
	if previous.Sign() != 0 {
		out["percent_change"] = Decimal(new(big.Rat).Mul(new(big.Rat).Quo(d, previous), big.NewRat(100, 1)))
	} else {
		out["reason"] = "zero_baseline"
	}
	return out
}
func Period(preset, since, until, zone string, now time.Time) (time.Time, time.Time, error) {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, time.Time{}, output.Fail("TIMEZONE_UNAVAILABLE", "Account timezone unavailable; cannot resolve reporting dates.", 8)
	}
	local := now.In(loc)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	if preset == "" {
		start, e1 := time.ParseInLocation("2006-01-02", since, loc)
		end, e2 := time.ParseInLocation("2006-01-02", until, loc)
		if e1 != nil || e2 != nil || end.Before(start) {
			return time.Time{}, time.Time{}, output.Fail("USAGE", "Require ordered reporting dates.", 2)
		}
		return start, end, nil
	}
	switch preset {
	case "yesterday":
		d := today.AddDate(0, 0, -1)
		return d, d, nil
	case "today":
		return today, today, nil
	case "this_month":
		return time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, loc), today.AddDate(0, 0, -1), nil
	case "last_month":
		first := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, loc)
		return first.AddDate(0, -1, 0), first.AddDate(0, 0, -1), nil
	}
	if strings.HasPrefix(preset, "last_") && strings.HasSuffix(preset, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(preset, "last_"), "d"))
		if err == nil && n > 0 && n <= 366 {
			return today.AddDate(0, 0, -n), today.AddDate(0, 0, -1), nil
		}
	}
	return time.Time{}, time.Time{}, output.Fail("USAGE", "Reports support today, yesterday, this_month, last_month and last_Nd; use explicit dates for other periods.", 2)
}
