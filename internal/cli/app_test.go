package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestLocalCommands(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"meta", "--help"}, {"version"}, {"config", "path"}} {
		var out, errOut bytes.Buffer
		if code := Run(context.Background(), args, &bytes.Buffer{}, &out, &errOut); code != 0 {
			t.Fatalf("%v: %d %s", args, code, errOut.String())
		}
		if out.Len() == 0 {
			t.Fatal("empty output")
		}
		if errOut.Len() != 0 {
			t.Fatal(errOut.String())
		}
	}
}
func TestUsageErrorIsStructured(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run(context.Background(), []string{"--unknown"}, &bytes.Buffer{}, &out, &errOut)
	if code != 2 || out.Len() != 0 {
		t.Fatal(code, out.String())
	}
	var e map[string]any
	if err := json.Unmarshal(errOut.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if e["ok"] != false {
		t.Fatal(e)
	}
}
