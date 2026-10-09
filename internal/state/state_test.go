package state

import (
	"os"
	"testing"
)

func TestPrivateStateAndLock(t *testing.T) {
	dir := t.TempDir()
	unlock, err := Lock(dir, "run_test")
	if err != nil {
		t.Fatal(err)
	}
	if release, err := Lock(dir, "run_test"); err == nil {
		release()
		t.Fatal("concurrent lock allowed")
	}
	unlock()
	unlock, err = Lock(dir, "run_test")
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	if _, err = Path(dir, "../escape"); err == nil {
		t.Fatal("traversal allowed")
	}
	if err = Save(dir, "run_test", map[string]string{"state": "unknown"}); err != nil {
		t.Fatal(err)
	}
	path, _ := Path(dir, "run_test")
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal(err, info)
	}
	var result map[string]string
	if err = Read(path, &result); err != nil || result["state"] != "unknown" {
		t.Fatal(err, result)
	}
}
