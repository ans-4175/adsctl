//go:build !windows

package state

import (
	"adsctl/internal/output"
	"os"
	"path/filepath"
	"syscall"
)

// Lock is released by the operating system if the process crashes.
func Lock(dir, key string) (func(), error) {
	path, err := Path(dir, key)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, output.Fail("STATE_LOCKED", "Another process is using this local run or monitor state.", 2)
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
