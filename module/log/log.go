package log

import (
	"io"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

type Config struct {
	StoreLogs bool   `default:"true"`
	LogDir    string `default:"kevlar/logs/"`
}

const (
	LogPerm = 0755
)

func Start(config Config) (*os.File, error) {
	var output io.Writer

	err := os.MkdirAll(config.LogDir, LogPerm)

	if err != nil {
		return nil, err
	}

	file, err := OpenLogFile(config)

	if err != nil {
		return nil, err
	}
	output = os.Stdout
	if config.StoreLogs {
		output = io.MultiWriter(file, os.Stdout)
	}

	logrus.StandardLogger().Level = logrus.TraceLevel
	logrus.StandardLogger().Formatter = &logrus.TextFormatter{}

	logrus.SetOutput(output)
	return file, nil
}

func OpenLogFile(config Config) (*os.File, error) {
	date := time.Now().Format("[02-01-2006]")
	LogPath := config.LogDir + "kevlar-" + date + ".log"
	return os.OpenFile(LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, LogPerm)
}
