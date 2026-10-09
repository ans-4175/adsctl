package cli

import "testing"

func TestSnapshotDiffRejectsIncompleteAndScopeChanges(t *testing.T) {
	snap := snapshot{Version: 1, Account: "act_1", APIVersion: "v25.0", Complete: true, Resources: map[string][]any{"campaigns": {map[string]any{"id": "123", "name": "before"}}, "ads": {}, "adsets": {}, "creatives": {}}}
	next := snap
	next.Complete = false
	if _, err := diffSnapshots(snap, next); err == nil {
		t.Fatal("incomplete accepted")
	}
	next = snap
	next.Account = "act_2"
	if _, err := diffSnapshots(snap, next); err == nil {
		t.Fatal("cross-account accepted")
	}
	next = snap
	next.Resources = map[string][]any{"campaigns": {map[string]any{"id": "123", "name": "after"}}, "ads": {}, "adsets": {}, "creatives": {}}
	changes, err := diffSnapshots(snap, next)
	if err != nil || len(changes.([]any)) != 1 {
		t.Fatal(err, changes)
	}
}
