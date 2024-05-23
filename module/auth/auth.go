package auth

import (
	"errors"
	"fmt"
	mongodb "kevlar/module/db/mongo"
	"kevlar/module/sec"
	"strings"
	"time"

	"github.com/rivo/uniseg"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
)

const (
	SessionExpiry = 6 * time.Hour
	ReloginExpiry = 15 * 24 * time.Hour
)

var (
	ErrIncorrectInputLength = errors.New("invalid input length")
	ErrInvalidCharacter     = errors.New("invalid character in input")
	ErrAccountDisabled      = errors.New("account is disabled")
	ErrLoginIncorrect       = errors.New("login details incorrect")
)

type Config struct {
	Domain           string `default:"example.com"`
	InputLengthCheck bool   `default:"true"`
	MaxInputLength   int    `default:"30"`
	MinInputLength   int    `default:"7"`
}

type Auth struct {
	*mongodb.MongoClient
	Config
}

func New(mongo *mongodb.MongoClient, config Config) Auth {
	return Auth{
		MongoClient: mongo,
		Config:      config,
	}
}

type User struct {
	UserID   string `bson:"userID"`
	Username string `bson:"username"`
	Password []byte `bson:"hash"`
	Disabled bool   `bson:"disabled"`
}
type Relogin struct {
	Relogin   string    `bson:"relogin"`
	ExpiresAt time.Time `bson:"expiresAt"`
	UserID    string    `bson:"userID"`
}

func checkUserID(userID string) bool {

	allowed := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890_-"

	for _, letter := range userID {
		found := strings.ContainsRune(allowed, letter)
		if !found {
			return false
		}
	}
	return true
}
func checkPassword(password string) bool {

	allowed := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ~`!1@2#3$4%5^6&7*8(9)0_-+={[}]:;<,>.?/|"

	for _, letter := range password {
		found := strings.ContainsRune(allowed, letter)
		if !found {
			return false
		}
	}
	return true
}

// Generates an relogin key, replaces if already exists.
func (auth Auth) newRelogin(userID string) (string, error) {
	context, cancel := auth.DefaultContext()
	defer cancel()

	collection := auth.Database(mongodb.Users).Collection(mongodb.Relogin)

	relogin := Relogin{
		Relogin:   sec.RandStr(64),
		ExpiresAt: time.Now().Add(ReloginExpiry),
		UserID:    userID,
	}

	result := collection.FindOne(context, bson.D{
		{Key: "userID", Value: userID},
	})

	if result.Err() == mongo.ErrNoDocuments {
		_, err := collection.InsertOne(context, relogin)
		if err != nil {
			return "", err
		}
	} else if result.Err() == nil {
		result = collection.FindOneAndReplace(context, bson.D{
			{Key: "userID", Value: userID},
		}, relogin)
		if result.Err() != nil {
			return "", result.Err()
		}
	} else {
		return "", result.Err()
	}
	return relogin.Relogin, nil
}

func (auth Auth) Disabled(userID string) error {
	context, cancel := auth.DefaultContext()
	defer cancel()

	collection := auth.Database(mongodb.Users).Collection(mongodb.Accounts)

	var user User

	err := collection.FindOne(context, bson.D{
		{Key: "userID", Value: userID},
	}).Decode(&user)

	if err != nil {
		return err
	}
	if user.Disabled {
		return ErrAccountDisabled
	}

	return nil
}

func (auth Auth) Create(userID, username, password string) (string, error) {
	context, cancel := auth.DefaultContext()
	defer cancel()

	wrapper := func(err error) error { return fmt.Errorf("[auth][%s]error while creating account: %w", userID, err) }

	collection := auth.Database(mongodb.Users).Collection(mongodb.Accounts)

	if auth.Config.InputLengthCheck {
		userIdCheck := !(uniseg.GraphemeClusterCount(userID) < auth.Config.MinInputLength || uniseg.GraphemeClusterCount(userID) > auth.Config.MaxInputLength)
		usernameCheck := !(uniseg.GraphemeClusterCount(username) < auth.Config.MinInputLength || uniseg.GraphemeClusterCount(username) > auth.Config.MaxInputLength)
		passwordCheck := !(uniseg.GraphemeClusterCount(password) < auth.Config.MinInputLength || uniseg.GraphemeClusterCount(password) > auth.Config.MaxInputLength)

		characterCheck := (checkUserID(userID) && checkPassword(password))

		if !userIdCheck || !usernameCheck || !passwordCheck {
			return "", wrapper(ErrIncorrectInputLength)
		}
		if !characterCheck {
			return "", wrapper(ErrInvalidCharacter)
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", wrapper(err)
	}

	user := User{
		UserID:   userID,
		Username: username,
		Password: hash,
		Disabled: false,
	}
	_, err = collection.InsertOne(context, user)
	if err != nil {
		return "", wrapper(err)
	}
	relogin, err := auth.newRelogin(userID)
	if err != nil {
		return "", wrapper(err)
	}
	return relogin, nil
}

func (auth Auth) Login(userID, password string) (string, error) {
	context, cancel := auth.DefaultContext()
	defer cancel()

	wrapper := func(err error) error { return fmt.Errorf("[auth][%s]error while logging in: %w", userID, err) }

	collection := auth.Database(mongodb.Users).Collection(mongodb.Accounts)

	if err := auth.Disabled(userID); err != nil {
		return "", wrapper(err)
	}

	var user User

	err := collection.FindOne(context, bson.D{
		{Key: "userID", Value: userID},
	}).Decode(&user)

	if err != nil {
		return "", wrapper(err)
	}
	if bcrypt.CompareHashAndPassword(user.Password, []byte(password)) == nil {
		return auth.newRelogin(userID)
	}
	return "", wrapper(ErrLoginIncorrect)
}

func (auth Auth) Logout(userID, relogin string) error {
	context, cancel := auth.DefaultContext()
	defer cancel()

	wrapper := func(err error) error { return fmt.Errorf("[auth][%s]error while logging out: %w", userID, err) }

	reloginCollection := auth.Database(mongodb.Users).Collection(mongodb.Relogin)

	if err := auth.Disabled(userID); err != nil {
		return wrapper(err)
	}

	var reloginEntry Relogin
	err := reloginCollection.FindOne(context, bson.D{
		{Key: "relogin", Value: relogin},
	}).Decode(&reloginEntry)

	if err != nil {
		return wrapper(err)
	}
	_, err = reloginCollection.DeleteOne(context, bson.D{
		{Key: "userID", Value: userID},
	})
	if err != nil {
		return wrapper(err)
	}
	return nil
}

func (auth Auth) Autologin(reloginUser string) (string, string, error) {
	context, cancel := auth.DefaultContext()
	defer cancel()

	wrapper := func(err error) error {
		return fmt.Errorf("[auth]error while logging in user with relogin key: %w", err)
	}

	collection := auth.Database(mongodb.Users).Collection(mongodb.Relogin)

	var relogin Relogin

	err := collection.FindOne(context, bson.D{
		{Key: "relogin", Value: reloginUser},
	}).Decode(&relogin)
	if err != nil {
		return "", "", wrapper(err)
	}
	if err := auth.Disabled(relogin.UserID); err != nil {
		return "", "", wrapper(err)
	}
	if relogin.Relogin == reloginUser {
		data, err := auth.newRelogin(relogin.UserID)
		return relogin.UserID, data, err
	}
	return "", "", wrapper(ErrLoginIncorrect)
}

func (auth Auth) Delete(userID, password string) error {
	context, cancel := auth.DefaultContext()
	defer cancel()

	wrapper := func(err error) error { return fmt.Errorf("[auth][%s]error while deleting user: %w", userID, err) }

	collection := auth.Database(mongodb.Users).Collection(mongodb.Accounts)

	if err := auth.Disabled(userID); err != nil {
		return wrapper(err)
	}

	var user User
	err := collection.FindOne(context, bson.D{
		{Key: "userID", Value: userID},
	}).Decode(&user)

	if err != nil {
		return wrapper(err)
	}
	err = bcrypt.CompareHashAndPassword(user.Password, []byte(password))
	if err != nil {
		return wrapper(err)
	}

	reloginCollection := auth.Database(mongodb.Users).Collection(mongodb.Relogin)

	_, err = reloginCollection.DeleteOne(context, bson.D{
		{Key: "userID", Value: userID},
	})
	if err != nil {
		return wrapper(err)
	}
	_, err = collection.DeleteOne(context, bson.D{
		{Key: "userID", Value: userID},
	})
	if err != nil {
		return wrapper(err)
	}
	return nil
}
