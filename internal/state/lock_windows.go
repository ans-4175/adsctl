//go:build windows

package state

import "adsctl/internal/output"

func Lock(dir, key string) (func(), error) {
	return nil, output.Fail("UNSUPPORTED", "Persistent mutations and monitor state currently require macOS or Linux file locking.", 2)
}
