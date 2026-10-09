package document

import "testing"

func TestStrictSingleDocument(t *testing.T) {
	var value struct {
		Version int `yaml:"version"`
	}
	for _, raw := range []string{"version: 1\nsecret: x", "version: 1\n---\nversion: 2", "[broken"} {
		if Decode([]byte(raw), &value) == nil {
			t.Fatal("accepted", raw)
		}
	}
	if err := Decode([]byte(`{"version":1}`), &value); err != nil || value.Version != 1 {
		t.Fatal(err, value)
	}
}
