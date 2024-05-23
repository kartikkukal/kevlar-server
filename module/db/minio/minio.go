package minio

import (
	"context"
	"kevlar/module/sec"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/sirupsen/logrus"
)

type MinioClient struct {
	*minio.Client
	config Config
}

type Config struct {
	MinioURI               string `default:"127.0.0.1:9000"`
	AccessKey              string
	SecretKey              string
	RetryConnection        bool `default:"true"`
	MaxRetry               int  `default:"3"`
	RequestTimeoutDuration int  `default:"40"`
	UseSSL                 bool `default:"false"`
}

func New(config Config) MinioClient {
	return MinioClient{config: config}
}

func (client *MinioClient) DefaultContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(),
		time.Duration(client.config.RequestTimeoutDuration)*time.Second)

}

func (client *MinioClient) CheckConnection() error {
	context, cancel := client.DefaultContext()
	defer cancel()
	str := sec.RandStrAlphabet(10)
	err := client.MakeBucket(context, "status-check-"+str, minio.MakeBucketOptions{})
	if err != nil {
		return err
	}
	err = client.RemoveBucket(context, "status-check-"+str)
	if err != nil {
		return err
	}
	return nil
}

func (client *MinioClient) Connect() error {
	log := logrus.WithField("URI", client.config.MinioURI)
	attempt := func() error {
		log.Trace("attempting connection to minio")
		var err error
		client.Client, err = minio.New(client.config.MinioURI, &minio.Options{
			Creds:  credentials.NewStaticV4(client.config.AccessKey, client.config.SecretKey, ""),
			Secure: client.config.UseSSL,
		})
		if err != nil {
			return err
		}
		err = client.CheckConnection()
		if err != nil {
			return err
		}
		log.Trace("connected to minio")
		return nil
	}
	if client.config.RetryConnection {
		var err error
		for retries := 0; retries < client.config.MaxRetry; retries++ {
			err = attempt()
			if err == nil {
				return nil
			} else {
				log.Trace("retrying connection...")
			}
		}
		log.Trace("retry limit reached")
		return err
	} else {
		err := attempt()
		return err
	}
}
