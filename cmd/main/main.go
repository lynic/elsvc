package main

import (
	"os"

	"github.com/lynic/elsvc"

	"github.com/sirupsen/logrus"
)

func main() {
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.DebugLevel)
	err := elsvc.StartGOPlugin("")
	if err != nil {
		logrus.Error(err)
		return
	}
}
