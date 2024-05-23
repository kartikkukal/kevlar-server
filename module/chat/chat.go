package chat

import (
	"errors"
	"fmt"
	"kevlar/module/attr"
	"kevlar/module/db/mongo"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	ContactList  = "chat_contact_list"
	IncomingList = "chat_incoming_list"
	OutgoingList = "chat_outgoing_list"
	LastSeen     = "last_seen"
	Username     = "username"
	About        = "about"

	WebPushSubscription = "web_push_subscription"

	MessageIncoming = "message_incoming"

	MaxResults = 49
)

var (
	ErrContactDoesNotExist = errors.New("contact doesn't exist")
	ErrMessageBlank        = errors.New("message is blank")
)

type Chat struct {
	*mongo.MongoClient
	*attr.Attr
}

type User struct {
	UserID   string `bson:"userID" json:"userID"`
	Username string `bson:"username" json:"username"`

	Message string `bson:"message" json:"message"`

	Store string `bson:"store" json:"-"`
}

type Message struct {
	ID string `bson:"id" json:"id"`

	From string `bson:"from" json:"from"`

	Type string `bson:"type" json:"type"`
	Data string `bson:"data" json:"data"`

	Read bool `bson:"read" json:"read"`

	Time time.Time `bson:"time" json:"time"`
}

func New(attr attr.Attr, mongo *mongo.MongoClient) Chat {
	return Chat{
		Attr:        &attr,
		MongoClient: mongo,
	}
}

func (chat Chat) StoreMessage(message_type, message_data, from, to string, online bool) error {

	context, cancel := chat.DefaultContext()
	defer cancel()

	from_user, err := chat.GetInformation(from)
	if err != nil {
		return err
	}

	var from_contacts []User

	err = chat.GetAttribute(from, ContactList, &from_contacts)
	if err != nil {
		return err
	}

	var to_contacts []User

	err = chat.GetAttribute(to, ContactList, &to_contacts)
	if err != nil {
		return err
	}

	subline := fmt.Sprintf("%s: %s", from_user.Username, message_data)

	var store string
	var found bool

	for index, value := range from_contacts {
		if value.UserID == to {
			from_contacts[index].Message = subline
			store = value.Store
			found = true
		}
	}
	if !found {
		return ErrContactDoesNotExist
	}

	found = false

	for index, value := range to_contacts {
		if value.UserID == from {
			to_contacts[index].Message = subline
			found = true
		}
	}
	if !found {
		return ErrContactDoesNotExist
	}

	err = chat.SetAttribute(from, ContactList, from_contacts)
	if err != nil {
		return err
	}

	err = chat.SetAttribute(to, ContactList, to_contacts)
	if err != nil {
		return err
	}

	trimmed_text := strings.TrimSpace(message_data)
	if trimmed_text == "" {
		return ErrMessageBlank
	}

	collection := chat.Database(mongo.Chat).Collection(store)

	_, err = collection.InsertOne(context, Message{
		ID:   uuid.New().String(),
		From: from,
		Type: message_type,
		Data: message_data,
		Read: online,
		Time: time.Now(),
	})
	return err
}

func (chat Chat) MarkRead(from, to string) error {
	context, cancel := chat.DefaultContext()
	defer cancel()

	var contacts []User

	err := chat.GetAttribute(from, ContactList, &contacts)
	if err != nil {
		return err
	}

	var contact User
	var found bool

	for _, value := range contacts {
		if value.UserID == to {
			contact = value
			found = true
		}
	}
	if !found {
		return ErrContactDoesNotExist
	}

	options := options.Find()
	options.Sort = bson.D{{Key: "time", Value: -1}}

	collection := chat.Database(mongo.Chat).Collection(contact.Store)

	cursor, err := collection.Find(context, bson.D{}, options)
	if err != nil {
		return err
	}

	for cursor.Next(context) {
		var message Message

		err = cursor.Decode(&message)

		if err != nil {
			return err
		}

		if message.From == to {
			if message.Read {
				break
			}
			message.Read = true
		}

		err = collection.FindOneAndReplace(context, bson.D{{Key: "id", Value: message.ID}}, message).Err()

		if err != nil {
			return err
		}
	}

	return nil
}

func (chat Chat) LoadMessages(from, to string, since int) ([]Message, error) {
	context, cancel := chat.DefaultContext()
	defer cancel()

	var contacts []User

	err := chat.GetAttribute(from, ContactList, &contacts)
	if err != nil {
		return nil, err
	}

	var contact User
	var found bool

	for _, value := range contacts {
		if value.UserID == to {
			contact = value
			found = true
		}
	}
	if !found {
		return nil, ErrContactDoesNotExist
	}

	err = chat.MarkRead(from, to)
	if err != nil {
		return nil, err
	}

	options := options.Find()
	options.Sort = bson.D{{Key: "time", Value: -1}}

	collection := chat.Database(mongo.Chat).Collection(contact.Store)

	cursor, err := collection.Find(context, bson.D{}, options)
	if err != nil {
		return nil, err
	}

	var messages []Message

	defer cursor.Close(context)

	for i := 0; cursor.Next(context) && i <= (since+MaxResults); i++ {
		var message Message
		err = cursor.Decode(&message)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}

	return messages[since:], nil
}
