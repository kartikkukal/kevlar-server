package attr

import (
	"bytes"
	"encoding/gob"
	"errors"
	"kevlar/module/db/mongo"
	"kevlar/module/sec"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

var (
	ErrSessionExpired      = errors.New("session has expired")
	ErrSessionDoesNotExist = errors.New("session does not exist")
	ErrKeyDoesNotExist     = errors.New("key does not exist in user attributes")
	ErrInvalidType         = errors.New("invalid type interface")
)

type Config struct {
	SessionExpiry int `default:"6"`
}

type Attr struct {
	*mongo.MongoClient
	Config
}

type Attributes struct {
	UserID     string            `bson:"userID"`
	Attributes map[string][]byte `bson:"attributes"`
}

func New(mongo *mongo.MongoClient, config Config) Attr {
	return Attr{
		MongoClient: mongo,
		Config:      config,
	}
}

// Creates a user entry with session and disabled value and returns the generated session
func (attr Attr) CreateUser(userID string) error {
	context, cancel := attr.DefaultContext()
	defer cancel()

	attributes := Attributes{
		UserID:     userID,
		Attributes: make(map[string][]byte),
	}

	collection := attr.Database(mongo.Users).Collection(mongo.Attributes)

	_, err := collection.InsertOne(context, attributes)
	if err != nil {
		return err
	}

	err = attr.SetAttribute(userID, "disabled", false)

	return err
}

// Creates a session with expiry
func (attr Attr) CreateSession(userID string) (string, error) {
	session := sec.RandStr(32)
	expiry := time.Now().Add(time.Hour * time.Duration(attr.SessionExpiry))

	err := attr.SetAttribute(userID, "session", session)
	if err != nil {
		return "", err
	}

	err = attr.SetAttribute(userID, "expiry", expiry)
	if err != nil {
		return "", err
	}

	return session, nil
}

func (attr Attr) VerifySession(userID, session_check string) error {

	var expiry time.Time
	var session string

	err := attr.GetAttribute(userID, "expiry", &expiry)
	if err != nil {
		return err
	}

	err = attr.GetAttribute(userID, "session", &session)
	if err != nil {
		return err
	}

	if !time.Now().Before(expiry) || session_check != session {
		return ErrSessionExpired
	}

	return nil
}

func (attr Attr) DeleteSession(userID string) error {

	err := attr.DeleteAttribute(userID, "session")
	if err != nil {
		return err
	}

	err = attr.DeleteAttribute(userID, "expiry")

	return err
}

func (attr Attr) SetAttribute(userID, key string, value interface{}) error {
	context, cancel := attr.DefaultContext()
	defer cancel()

	var attributes Attributes

	collection := attr.Database(mongo.Users).Collection(mongo.Attributes)

	err := collection.FindOne(context, bson.D{
		{Key: "userID", Value: userID},
	}).Decode(&attributes)
	if err != nil {
		return err
	}

	buffer := new(bytes.Buffer)
	encoder := gob.NewEncoder(buffer)

	err = encoder.Encode(value)
	if err != nil {
		return err
	}

	attributes.Attributes[key] = buffer.Bytes()

	err = collection.FindOneAndReplace(context, bson.D{
		{Key: "userID", Value: userID},
	}, attributes).Err()

	return err
}

func (attr Attr) GetAttribute(userID, key string, value interface{}) error {
	context, cancel := attr.DefaultContext()
	defer cancel()

	collection := attr.Database(mongo.Users).Collection(mongo.Attributes)

	var attributes Attributes

	err := collection.FindOne(context, bson.D{
		{Key: "userID", Value: userID},
	}).Decode(&attributes)
	if err != nil {
		return err
	}

	data, ok := attributes.Attributes[key]
	if !ok {
		return ErrKeyDoesNotExist
	}

	buffer := bytes.NewBuffer(data)
	decoder := gob.NewDecoder(buffer)

	err = decoder.Decode(value)
	if err != nil {
		return err
	}

	return nil
}
func (attr Attr) DeleteAttribute(userID, key string) error {
	context, cancel := attr.DefaultContext()
	defer cancel()

	var attributes Attributes

	collection := attr.Database(mongo.Users).Collection(mongo.Attributes)

	err := collection.FindOne(context, bson.D{
		{Key: "userID", Value: userID},
	}).Decode(&attributes)
	if err != nil {
		return err
	}

	delete(attributes.Attributes, key)

	err = collection.FindOneAndReplace(context, bson.D{
		{Key: "userID", Value: userID},
	}, attributes).Err()

	return err
}

func (attr Attr) DeleteUser(userID string) error {
	context, cancel := attr.DefaultContext()
	defer cancel()

	collection := attr.Database(mongo.Users).Collection(mongo.Attributes)

	_, err := collection.DeleteOne(context, bson.D{
		{Key: "userID", Value: userID},
	})

	return err
}
