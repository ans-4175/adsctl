package schema

import "fmt"

// MatchesPayload checks planned fields against normalized native read fields.
func MatchesPayload(expected, observed map[string]any) bool {
	for k, v := range expected {
		actual, ok := observed[k]
		if k == "creative" {
			want, wok := v.(map[string]any)
			got, gok := actual.(map[string]any)
			if wok && gok {
				id := got["id"]
				if id == nil {
					id = got["creative_id"]
				}
				if fmt.Sprint(want["creative_id"]) != fmt.Sprint(id) {
					return false
				}
				continue
			}
		}
		if !ok || !matchesValue(v, actual) {
			return false
		}
	}
	return true
}
func matchesValue(want, got any) bool {
	switch v := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		return ok && MatchesPayload(v, g)
	case []any:
		g, ok := got.([]any)
		if !ok || len(v) != len(g) {
			return false
		}
		for i := range v {
			if !matchesValue(v[i], g[i]) {
				return false
			}
		}
		return true
	default:
		return fmt.Sprint(want) == fmt.Sprint(got)
	}
}
