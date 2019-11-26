package main

import (
	"github.com/lynic/elsvc"
)

func main() {
	elsvc.SetupLogger("main", elsvc.LogDebugLevel)
	err := elsvc.StartService("")
	if err != nil {
		elsvc.Error("start go plugin error: %s", err.Error())
		return
	}
}
