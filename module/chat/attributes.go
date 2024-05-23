package chat

import "time"

func (chat Chat) GetAttributes(userID string, isOnline func(string) bool) (interface{}, error) {

	type Contact struct {
		User
		Online   bool  `json:"online"`
		LastSeen int64 `json:"last_seen,omitempty"`
	}

	var contacts []Contact

	err := chat.GetAttribute(userID, ContactList, &contacts)
	if err != nil {
		return nil, err
	}

	if contacts == nil {
		contacts = []Contact{}
	}

	for index, contact := range contacts {
		contacts[index].Online = isOnline(contact.UserID)

		var seen int64

		err = chat.GetAttribute(contact.UserID, LastSeen, &seen)
		if err != nil {
			chat.SetAttribute(contact.UserID, LastSeen, time.Now().Unix())
		}

		contacts[index].LastSeen = seen
	}

	var outgoing []User
	err = chat.GetAttribute(userID, OutgoingList, &outgoing)
	if err != nil {
		return nil, err
	}

	if outgoing == nil {
		outgoing = []User{}
	}

	for index, value := range outgoing {
		var about string

		err = chat.GetAttribute(value.UserID, About, &about)
		if err != nil {
			return nil, err
		}

		outgoing[index].Message = about
	}

	var incoming []User
	err = chat.GetAttribute(userID, IncomingList, &incoming)
	if err != nil {
		return nil, err
	}

	if incoming == nil {
		incoming = []User{}
	}

	for index, value := range incoming {
		var about string

		err = chat.GetAttribute(value.UserID, About, &about)
		if err != nil {
			return nil, err
		}

		incoming[index].Message = about
	}

	user, err := chat.GetInformation(userID)
	if err != nil {
		return nil, err
	}

	return struct {
		Contacts []Contact `json:"chat_contact_list"`
		Outgoing []User    `json:"chat_outgoing_list"`
		Incoming []User    `json:"chat_incoming_list"`

		Username string `json:"username"`
		UserID   string `json:"userID"`
		About    string `json:"about"`
	}{
		Contacts: contacts,
		Outgoing: outgoing,
		Incoming: incoming,

		Username: user.Username,
		UserID:   userID,
		About:    user.Message,
	}, nil
}
