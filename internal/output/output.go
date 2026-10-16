// Package output defines the stable machine interface shared by commands.
package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
)

type Error struct {
	Code            string `json:"code"`
	Message         string `json:"message"`
	Retryable       bool   `json:"retryable"`
	HTTPStatus      int    `json:"http_status,omitempty"`
	Provider        any    `json:"provider,omitempty"`
	SuggestedAction string `json:"suggested_action,omitempty"`
	Exit            int    `json:"-"`
}

func (e *Error) Error() string { return e.Message }
func Fail(code, message string, exit int) *Error {
	return &Error{Code: code, Message: message, Exit: exit}
}
func AsError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return Fail("INTERNAL", "Unexpected internal error.", 1)
}

type Envelope struct {
	SchemaVersion string         `json:"schema_version"`
	OK            bool           `json:"ok"`
	Data          any            `json:"data"`
	Meta          map[string]any `json:"meta"`
	Warnings      []Warning      `json:"warnings"`
	Error         *Error         `json:"error"`
}
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func New(data any, meta map[string]any) Envelope {
	if meta == nil {
		meta = map[string]any{}
	}
	return Envelope{SchemaVersion: "1", OK: true, Data: data, Meta: meta, Warnings: []Warning{}}
}
func Write(w io.Writer, format string, e Envelope) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(e)
	case "jsonl":
		enc := json.NewEncoder(w)
		if err := enc.Encode(map[string]any{"type": "meta", "meta": e.Meta, "schema_version": "1"}); err != nil {
			return err
		}
		rows, ok := e.Data.([]any)
		if !ok {
			rows = []any{e.Data}
		}
		for _, row := range rows {
			if err := enc.Encode(map[string]any{"type": "data", "data": row}); err != nil {
				return err
			}
		}
		if e.Error != nil {
			if err := enc.Encode(map[string]any{"type": "error", "error": e.Error}); err != nil {
				return err
			}
		}
		return enc.Encode(map[string]any{"type": "end", "ok": e.OK, "meta": e.Meta, "warnings": e.Warnings, "count": len(rows)})
	case "table":
		return table(w, e)
	default:
		return Fail("USAGE", "Output must be json, jsonl or table.", 2)
	}
}
func table(w io.Writer, e Envelope) error {
	rows, ok := e.Data.([]any)
	if !ok {
		rows = []any{e.Data}
	}
	keys := map[string]bool{}
	for _, row := range rows {
		if obj, ok := row.(map[string]any); ok {
			for k := range obj {
				keys[k] = true
			}
		}
	}
	columns := make([]string, 0, len(keys))
	for k := range keys {
		columns = append(columns, k)
	}
	sort.Strings(columns)
	if len(columns) == 0 {
		b, err := json.MarshalIndent(e.Data, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(b))
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, strings.Join(columns, "\t")); err != nil {
		return err
	}
	for _, row := range rows {
		obj, ok := row.(map[string]any)
		if !ok {
			continue
		}
		cells := make([]string, len(columns))
		for i, k := range columns {
			b, err := json.Marshal(obj[k])
			if err != nil {
				return err
			}
			cells[i] = strings.ReplaceAll(string(b), "\t", " ")
		}
		if _, err := fmt.Fprintln(tw, strings.Join(cells, "\t")); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "%d rows | complete=%v | account=%v\n", len(rows), e.Meta["complete"], e.Meta["account_id"])
	return err
}
