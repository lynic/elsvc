package elsvc

import (
	"os"
	"runtime"
	"time"
)

// IsDir reports whether the dir exists as a boolean
func IsDir(name string) bool {
	if fi, err := os.Stat(name); err == nil {
		if fi.Mode().IsDir() {
			return true
		}
	}
	return false
}

// IsFile reports whether the named file exists as a boolean
func IsFile(name string) bool {
	if fi, err := os.Stat(name); err == nil {
		if fi.Mode().IsRegular() {
			return true
		}
	}
	return false
}

func WaitGoroutines(num int) {
	for runtime.NumGoroutine() > num {
		logger.Debug("wait goruntines: current=%d > expect=%d", runtime.NumGoroutine(), num)
		time.Sleep(1 * time.Second)
	}
}
