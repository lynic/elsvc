package elsvc

import (
	"os"
	"runtime"
	"time"
)

// IsDir reports whether the dir exists as a boolean
func isDir(name string) bool {
	if fi, err := os.Stat(name); err == nil {
		if fi.Mode().IsDir() {
			return true
		}
	}
	return false
}

// IsFile reports whether the named file exists as a boolean
func isFile(name string) bool {
	if fi, err := os.Stat(name); err == nil {
		if fi.Mode().IsRegular() {
			return true
		}
	}
	return false
}

func waitGoroutines(num int) {
	for runtime.NumGoroutine() > num {
		logger.Debug("wait goruntines: current=%d > expect=%d", runtime.NumGoroutine(), num)
		time.Sleep(1 * time.Second)
	}
}
