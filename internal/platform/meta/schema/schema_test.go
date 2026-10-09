package schema

import "testing"

func TestMutationSchemaRejectsUnsafePayloads(t *testing.T) {
	for _, input := range []string{`{"access_token":"secret"}`, `{"status":"DELETED"}`, `{"name":"x","adset_id":"123","creative":{"object_story_spec":{}}}`, `{"name":"x","daily_budget":2.5}`, `{"name":"x","daily_budget":100,"lifetime_budget":200}`} {
		p, err := Decode([]byte(input))
		if err != nil {
			t.Fatal(err)
		}
		if Validate("ads", "create", p) == nil {
			t.Fatal("accepted", input)
		}
	}
	for _, input := range []string{`{} {}`, `{} garbage`, `[]`, `null`} {
		if _, err := Decode([]byte(input)); err == nil {
			t.Fatal("accepted", input)
		}
	}
}
func TestExactIntegerPayload(t *testing.T) {
	p, err := Decode([]byte(`{"name":"x","daily_budget":9007199254740993}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = Validate("campaigns", "update", p); err != nil {
		t.Fatal(err)
	}
	if p["daily_budget"].(interface{ String() string }).String() != "9007199254740993" {
		t.Fatal(p)
	}
}
