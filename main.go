package main

import (
	"kevlar/module/start"

	"github.com/sirupsen/logrus"
)

func main() {
	err := start.Start()
	if err != nil {
		logrus.WithError(err).Trace("error while starting server")
	}
}
