package chat

import (
	"errors"
	"kevlar/module/db/mongo"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ErrUserIDExists = errors.New("userID already exists")
	ErrInvalidQuery = errors.New("invalid query provided")
	ErrInvalidCast  = errors.New("invalid type cast")
)

func (chat Chat) CreateUser(userID string) error {

	err := chat.SetAttribute(userID, ContactList, []User{})
	if err != nil {
		return err
	}

	err = chat.SetAttribute(userID, IncomingList, []User{})
	if err != nil {
		return err
	}

	err = chat.SetAttribute(userID, OutgoingList, []User{})
	if err != nil {
		return err
	}

	err = chat.SetAttribute(userID, About, "Hello! I am a new user on kevlar!")
	if err != nil {
		return err
	}

	err = chat.SetAttribute(userID, LastSeen, time.Now().Unix())

	return err
}

func (chat Chat) Search(from, query string, last string) ([]User, string, error) {
	context, cancel := chat.DefaultContext()
	defer cancel()

	if query == "*" {
		return nil, "", ErrInvalidQuery
	}

	collection := chat.Database(mongo.Users).Collection(mongo.Accounts)

	options := options.Find().SetProjection(bson.D{
		{Key: "userID", Value: 1},
		{Key: "_id", Value: 1},
	}).SetLimit(MaxResults)
	options.Sort = bson.D{{Key: "_id", Value: -1}}

	var sort bson.D

	last_id, err := primitive.ObjectIDFromHex(last)
	if err != nil {
		last_id = primitive.NilObjectID
	}

	if last_id == primitive.NilObjectID {
		sort = bson.D{
			{Key: "userID", Value: primitive.Regex{
				Pattern: "^" + query,
				Options: "i",
			}},
		}
	} else {
		sort = bson.D{
			{Key: "userID", Value: primitive.Regex{
				Pattern: "^" + query,
				Options: "i",
			}},
			{Key: "_id", Value: bson.D{
				{Key: "$lt", Value: last_id},
			}},
		}
	}

	cursor, err := collection.Find(context, sort, options)
	if err != nil {
		return nil, "", err
	}

	var results []struct {
		ID     primitive.ObjectID `bson:"_id"`
		UserID string             `bson:"userID"`
	}

	err = cursor.All(context, &results)
	if err != nil {
		return nil, "", err
	}

	var new_last primitive.ObjectID

	if len(results) != 0 {
		new_last = results[len(results)-1].ID
	}

	var contacts []User
	err = chat.GetAttribute(from, ContactList, &contacts)
	if err != nil {
		return nil, "", err
	}

	var incoming []User
	err = chat.GetAttribute(from, IncomingList, &incoming)
	if err != nil {
		return nil, "", err
	}

	var outgoing []User
	err = chat.GetAttribute(from, OutgoingList, &outgoing)
	if err != nil {
		return nil, "", err
	}

	check_accounts := append(contacts, incoming...)
	check_accounts = append(check_accounts, outgoing...)

	var accounts []User

	for _, user := range results {

		var found bool

		for _, value := range check_accounts {
			if value.UserID == user.UserID || value.UserID == from {
				found = true
			}
		}

		if found || user.UserID == from {
			continue
		}

		user, err := chat.GetInformation(user.UserID)
		if err != nil {
			return nil, "", err
		}

		accounts = append(accounts, user)
	}

	if len(results) < MaxResults {
		return accounts, primitive.NilObjectID.Hex(), nil
	}

	return accounts, new_last.Hex(), nil
}

func (chat Chat) DeleteUser(userID string) error {
	context, cancel := chat.DefaultContext()
	defer cancel()

	remove_user := func(key string) error {

		var list []User
		err := chat.GetAttribute(userID, key, &list)
		if err != nil {
			return err
		}

		for _, value := range list {
			var list []User
			err := chat.GetAttribute(value.UserID, key, &list)
			if err != nil {
				return err
			}

			for index, value := range list {
				if value.UserID == userID {
					list = append(list[:index], list[index+1:]...)
					break
				}
			}

			err = chat.SetAttribute(value.UserID, key, list)
			if err != nil {
				return err
			}
		}
		return nil
	}

	err := remove_user(ContactList)
	if err != nil {
		return err
	}

	var contacts []User

	err = chat.GetAttribute(userID, ContactList, &contacts)
	if err != nil {
		return err
	}

	for _, value := range contacts {

		collection := chat.Database(mongo.Chat).Collection(value.Store)

		err = collection.Drop(context)
		if err != nil {
			return err
		}
	}

	err = remove_user(OutgoingList)
	if err != nil {
		return err
	}
	err = remove_user(IncomingList)

	return err
}
func (chat Chat) GetInformation(userID string) (User, error) {

	context, cancel := chat.DefaultContext()
	defer cancel()

	collection := chat.Database(mongo.Users).Collection(mongo.Accounts)

	options := options.FindOne().SetProjection(bson.D{
		{Key: "username", Value: 1},
	})

	var user struct {
		Username string `bson:"username"`
	}

	err := collection.FindOne(context, bson.D{
		{Key: "userID", Value: userID},
	}, options).Decode(&user)
	if err != nil {
		return User{}, err
	}

	var about string

	err = chat.GetAttribute(userID, About, &about)
	if err != nil {
		return User{}, err
	}

	return User{
		UserID:   userID,
		Username: user.Username,
		Message:  about,
	}, nil
}
