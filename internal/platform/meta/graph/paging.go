package graph

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strconv"

	"adsctl/internal/output"
)

type PageOptions struct {
	Scope, Cursor      string
	All                bool
	PageSize, MaxItems int
}
type ListResult struct {
	Rows     []any
	Complete bool
	Cursor   string
	Usage    map[string]any
}
type continuation struct {
	Fingerprint string `json:"fingerprint"`
	After       string `json:"after"`
	Pending     []any  `json:"pending,omitempty"`
	More        bool   `json:"more"`
}

func (c *Client) List(ctx context.Context, path string, params url.Values, o PageOptions) (ListResult, error) {
	r := ListResult{Rows: []any{}, Usage: map[string]any{}}
	if o.PageSize < 1 || o.PageSize > 1000 || o.MaxItems < 1 {
		return r, output.Fail("USAGE", "Page size must be 1..1000 and max-items positive.", 2)
	}
	q := clone(params)
	sum := sha256.Sum256([]byte(o.Scope + "|" + c.Version + "|" + path + "|" + q.Encode()))
	finger := hex.EncodeToString(sum[:])
	cp := continuation{Fingerprint: finger}
	if len(o.Cursor) > 8<<20 {
		return r, output.Fail("CURSOR_INVALID", "Cursor exceeds 8 MiB.", 2)
	}
	if o.Cursor != "" {
		b, err := base64.RawURLEncoding.DecodeString(o.Cursor)
		if err != nil {
			return r, output.Fail("CURSOR_INVALID", "Invalid continuation cursor.", 2)
		}
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.UseNumber()
		if err = dec.Decode(&cp); err != nil || cp.Fingerprint != finger {
			return r, output.Fail("CURSOR_INVALID", "Cursor does not match profile/account/version/query.", 2)
		}
	}
	finish := func(more bool) {
		r.Complete = !more
		if more {
			b, _ := json.Marshal(cp)
			r.Cursor = base64.RawURLEncoding.EncodeToString(b)
		}
	}
	seen := map[string]bool{}
	for {
		if len(cp.Pending) > 0 {
			n := min(len(cp.Pending), o.MaxItems-len(r.Rows))
			r.Rows = append(r.Rows, cp.Pending[:n]...)
			cp.Pending = cp.Pending[n:]
			if len(r.Rows) >= o.MaxItems || !o.All {
				finish(len(cp.Pending) > 0 || cp.More)
				return r, nil
			}
			if !cp.More {
				finish(false)
				return r, nil
			}
		}
		if seen[cp.After] {
			return r, output.Fail("PAGINATION_LOOP", "Provider repeated a continuation cursor.", 8)
		}
		seen[cp.After] = true
		q.Set("limit", strconv.Itoa(min(o.PageSize, o.MaxItems-len(r.Rows))))
		if cp.After != "" {
			q.Set("after", cp.After)
		}
		resp, err := c.Request(ctx, "GET", path, q)
		if err != nil {
			finish(true)
			return r, err
		}
		for k, v := range resp.Usage {
			r.Usage[k] = v
		}
		rows, ok := resp.Value["data"].([]any)
		if !ok {
			return r, output.Fail("RESPONSE_INVALID", "Expected a Graph data array.", 8)
		}
		for _, row := range rows {
			if _, ok := row.(map[string]any); !ok {
				return r, output.Fail("RESPONSE_INVALID", "Graph collection rows must be JSON objects.", 8)
			}
		}
		cp.After = ""
		cp.More = false
		if p, ok := resp.Value["paging"].(map[string]any); ok {
			if next, ok := p["next"].(string); ok && next != "" {
				cp.More = true
				if cursors, ok := p["cursors"].(map[string]any); ok {
					cp.After, _ = cursors["after"].(string)
				}
				if cp.After == "" {
					u, err := url.Parse(next)
					if err == nil {
						cp.After = u.Query().Get("after")
					}
				}
				if cp.After == "" {
					return r, output.Fail("RESPONSE_INVALID", "Provider pagination is missing a continuation cursor.", 8)
				}
			}
		}
		n := min(len(rows), o.MaxItems-len(r.Rows))
		r.Rows = append(r.Rows, rows[:n]...)
		cp.Pending = rows[n:]
		more := cp.More || len(cp.Pending) > 0
		if !o.All || !more || len(r.Rows) >= o.MaxItems {
			finish(more)
			return r, nil
		}
	}
}
