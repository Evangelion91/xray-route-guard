package guard

import (
	"errors"
	"os"
	"syscall"
)

func Lock(path string) (*os.File, error) {
	// Go opens files with close-on-exec: probe/restart children cannot retain it.
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, errors.New("another guard instance owns this configuration")
	}
	return f, nil
}
