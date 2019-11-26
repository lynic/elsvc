package main

import (
	"github.com/lynic/elsvc"
)

func main() {
	// logrus.SetFormatter(&logrus.TextFormatter{
	// 	FullTimestamp: true,
	// })
	// logrus.SetOutput(os.Stdout)
	// logrus.SetLevel(logrus.DebugLevel)
	elsvc.SetupLogger("elsvc", elsvc.LogDebugLevel)
	err := elsvc.StartService("")
	if err != nil {
		elsvc.Error("start go plugin error: %s", err.Error())
		return
	}
}
