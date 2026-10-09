// Package state stores private, bounded local receipts and execution journals.
package state

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"adsctl/internal/config"
	"adsctl/internal/output"
)

var safe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,79}$`)

func Dir(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "adsctl", "state"), nil
}
func Path(dir, key string) (string, error) {
	if !safe.MatchString(key) {
		return "", output.Fail("INPUT", "Invalid local state ID.", 2)
	}
	return filepath.Join(dir, key+".json"), nil
}
func Save(dir, key string, value any) error {
	path, err := Path(dir, key)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return config.Atomic(path, b)
}
func Read(path string, value any) error {
	f, err := os.Open(path)
	if err != nil {
		return output.Fail("INPUT", "Cannot open local state file.", 2)
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, 32<<20+1))
	if err != nil || len(b) > 32<<20 {
		return output.Fail("INPUT", "Local state exceeds 32 MiB or cannot be read.", 2)
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	d.DisallowUnknownFields()
	if err = d.Decode(value); err != nil {
		return output.Fail("INPUT", "Invalid local state document.", 2)
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return output.Fail("INPUT", "Expected one local state document.", 2)
	}
	return nil
}
func Exclusive(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return output.Fail("OUTPUT_EXISTS", "Output exists or cannot be created.", 2)
	}
	defer func() { _ = f.Close() }()
	if _, err = f.Write(b); err != nil {
		return err
	}
	return f.Sync()
}
