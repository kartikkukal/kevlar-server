package start

import (
	"kevlar/http"
	"kevlar/module/conf"
	"kevlar/module/db/minio"
	"kevlar/module/db/mongo"
	"kevlar/module/log"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
)

// This package handles the starting of the server.

func Start() error {
	// Load configuration
	config, err := conf.Read()
	if err != nil {
		logrus.WithError(err).Error("unable to load config")
		return err
	}
	// Starting logging
	file, err := log.Start(config.Log)
	if err != nil {
		logrus.WithError(err).Error("unable to open logfile")
		return err
	}

	// Connect to mongodb
	mongoClient := mongo.New(config.Mongo)
	err = mongoClient.Connect()
	if err != nil {
		logrus.WithError(err).Error("unable to connect to mongo")
		return err
	}
	minioClient := minio.New(config.Minio)
	err = minioClient.Connect()
	if err != nil {
		logrus.WithError(err).Error("unable to connect to minio")
		return err
	}

	// Start http server
	server := http.New(&mongoClient, &minioClient, config)
	go server.Start()

	// Set closing function
	Wait(func() {
		server.Close()
		mongoClient.Close()

		file.Close()
	})

	return nil
}
func Wait(deferred func()) {
	exit := make(chan os.Signal, 1)
	signal.Notify(exit, os.Interrupt, syscall.SIGTERM)
	<-exit
	logrus.Trace("shutting down server")
	deferred()
}
