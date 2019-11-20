package elsvc

import (
	"os"
	"runtime"
	"time"

	"github.com/sirupsen/logrus"
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

func WaitGoroutines() {
	for runtime.NumGoroutine() > 1 {
		logrus.Debugf("wait goruntines: %d", runtime.NumGoroutine())
		time.Sleep(1 * time.Second)
	}
}
