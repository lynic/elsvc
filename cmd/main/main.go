package main

import (
	"elsvc"
	"os"

	"github.com/sirupsen/logrus"
)

func main() {
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.DebugLevel)
	svc := &elsvc.Service{}
	logrus.Info("init service")
	err := svc.Init()
	if err != nil {
		logrus.Error(err)
		return
	}
	logrus.Info("start service")
	err = svc.Start()
	if err != nil {
		logrus.Error(err)
		return
	}
}
