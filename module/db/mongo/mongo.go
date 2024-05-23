package mongo

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoClient struct {
	*mongo.Client
	config Config
}

type Config struct {
	RequestTimeoutDuration int    `default:"30"`
	MongoURI               string `default:"mongodb://localhost:27017"`
	RetryConnection        bool   `default:"true"`
	MaxRetry               int    `default:"3"`
}

var (
	Users      = "user"
	Chat       = "chat"
	Accounts   = "accounts"
	Relogin    = "relogin"
	Attributes = "attr"
)

func New(config Config) MongoClient {
	return MongoClient{config: config}
}

func (db MongoClient) DefaultContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(),
		time.Duration(db.config.RequestTimeoutDuration)*time.Second)
}

func (db *MongoClient) InitializeDatabase() error {
	context, cancel := db.DefaultContext()
	defer cancel()
	uniqueFeild := func(feild string) mongo.IndexModel {
		return mongo.IndexModel{
			Keys: bson.M{
				feild: 1,
			},
			Options: options.Index().SetUnique(true),
		}
	}
	expirySet := mongo.IndexModel{
		Keys: bson.M{
			"expiresAt": 1,
		},
		Options: options.Index().SetExpireAfterSeconds(0),
	}
	uniqueUserID := uniqueFeild("userID")
	uniqueRelogin := uniqueFeild("relogin")

	accountsCollection := db.Database(Users).Collection(Accounts)
	reloginCollection := db.Database(Users).Collection(Relogin)
	attributesCollection := db.Database(Users).Collection(Attributes)

	_, err := accountsCollection.Indexes().CreateOne(context, uniqueUserID)
	if err != nil {
		return err
	}
	_, err = reloginCollection.Indexes().CreateMany(context, []mongo.IndexModel{uniqueUserID, uniqueRelogin, expirySet})
	if err != nil {
		return err
	}
	_, err = attributesCollection.Indexes().CreateOne(context, uniqueUserID)
	if err != nil {
		return err
	}

	return nil
}

func (db *MongoClient) Connect() error {
	log := logrus.WithField("URI", db.config.MongoURI)
	attempt := func() error {
		context, cancel := db.DefaultContext()
		defer cancel()

		log.Trace("attempting to connect to mongo")
		client, err := mongo.Connect(context, options.Client().ApplyURI(db.config.MongoURI))
		if err != nil {
			return err
		}

		err = client.Ping(context, nil)
		if err != nil {
			return err
		}

		db.Client = client
		err = db.InitializeDatabase()
		if err != nil {
			return err
		}
		log.Trace("connected to mongo")
		return nil
	}

	if db.config.RetryConnection {
		var err error
		for retries := 0; retries < db.config.MaxRetry; retries++ {
			err = attempt()
			if err == nil {
				return nil
			} else {
				log.Trace("retrying mongo connection...")
			}
		}
		return err
	} else {
		err := attempt()
		return err
	}
}

func (db *MongoClient) Close() {
	// Less time to shutdown
	context, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	db.Disconnect(context)

	logrus.Trace("mongodb disconnected")
}
