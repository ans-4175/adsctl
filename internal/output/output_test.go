package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONContractAndLosslessIDs(t *testing.T) {
	var out bytes.Buffer
	env := New([]any{map[string]any{"id": json.Number("9007199254740993"), "spend": "1.25"}}, map[string]any{"complete": false})
	if err := Write(&out, "json", env); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "9007199254740993") || !strings.Contains(out.String(), `"warnings": []`) {
		t.Fatal(out.String())
	}
	var decoded map[string]any
	d := json.NewDecoder(&out)
	d.UseNumber()
	if err := d.Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["schema_version"] != "1" || decoded["ok"] != true || decoded["error"] != nil {
		t.Fatal(decoded)
	}
}
func TestJSONLHasExplicitPartialTerminator(t *testing.T) {
	var out bytes.Buffer
	env := New([]any{map[string]any{"id": "1"}}, map[string]any{"complete": false})
	env.OK = false
	env.Error = Fail("INCOMPLETE", "bounded", 9)
	if err := Write(&out, "jsonl", env); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 {
		t.Fatal(out.String())
	}
	for i, kind := range []string{"meta", "data", "error", "end"} {
		var row map[string]any
		if err := json.Unmarshal([]byte(lines[i]), &row); err != nil || row["type"] != kind {
			t.Fatal(err, row)
		}
	}
	if !strings.Contains(lines[3], `"ok":false`) {
		t.Fatal(lines[3])
	}
}
func TestTableEscapesControlCharacters(t *testing.T) {
	var out bytes.Buffer
	if err := Write(&out, "table", New([]any{map[string]any{"name": "hello\n\x1b[31m"}}, map[string]any{"complete": true, "account_id": "act_1"})); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "\x1b") || !strings.Contains(out.String(), "account=act_1") {
		t.Fatal(out.String())
	}
}
