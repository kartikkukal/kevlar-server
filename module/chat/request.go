package chat

import (
	"errors"

	"github.com/google/uuid"
)

var (
	ErrUserDoesNotExist = errors.New("user not present in list")
	ErrRequestSent      = errors.New("request already sent")
	ErrSelfRequest      = errors.New("cannot send request to self")
	ErrRequestNotSent   = errors.New("no request sent")
)

func (chat Chat) Request(from, to string) error {

	var outgoing []User
	err := chat.GetAttribute(from, OutgoingList, &outgoing)
	if err != nil {
		return err
	}

	for _, user := range outgoing {
		if user.UserID == to {
			return ErrRequestSent
		}
	}

	var incoming []User
	err = chat.GetAttribute(to, IncomingList, &incoming)
	if err != nil {
		return err
	}

	from_user, err := chat.GetInformation(from)
	if err != nil {
		return err
	}

	to_user, err := chat.GetInformation(to)
	if err != nil {
		return err
	}

	incoming = append(incoming, from_user)
	outgoing = append(outgoing, to_user)

	err = chat.SetAttribute(to, IncomingList, incoming)
	if err != nil {
		return err
	}

	err = chat.SetAttribute(from, OutgoingList, outgoing)
	if err != nil {
		return err
	}

	return nil
}

func (chat Chat) Cancel(from, to string) error {

	var outgoing []User
	err := chat.GetAttribute(from, OutgoingList, &outgoing)
	if err != nil {
		return err
	}

	var incoming []User
	err = chat.GetAttribute(to, IncomingList, &incoming)
	if err != nil {
		return err
	}

	found := false

	for index, value := range outgoing {
		if value.UserID == to {
			outgoing = append(outgoing[:index], outgoing[index+1:]...)
			found = true
			break
		}
	}

	if !found {
		return ErrRequestNotSent
	}

	for index, value := range incoming {
		if value.UserID == from {
			incoming = append(incoming[:index], incoming[index+1:]...)
			found = true
			break
		}
	}

	if !found {
		return ErrRequestNotSent
	}

	err = chat.SetAttribute(to, IncomingList, incoming)
	if err != nil {
		return err
	}

	err = chat.SetAttribute(from, OutgoingList, outgoing)

	return err
}

func (chat Chat) Decide(from, to string, decide bool) error {

	err := chat.Cancel(to, from)
	if err != nil {
		return err
	}

	if decide {
		store := uuid.New().String()

		var from_contacts []User
		err = chat.GetAttribute(from, ContactList, &from_contacts)
		if err != nil {
			return err
		}

		to_user, err := chat.GetInformation(to)
		if err != nil {
			return err
		}

		to_user.Store = store

		from_contacts = append(from_contacts, to_user)

		err = chat.SetAttribute(from, ContactList, from_contacts)
		if err != nil {
			return err
		}

		var to_contacts []User
		err = chat.GetAttribute(to, ContactList, &to_contacts)
		if err != nil {
			return err
		}

		from_user, err := chat.GetInformation(from)
		if err != nil {
			return err
		}

		from_user.Store = store

		to_contacts = append(to_contacts, from_user)

		err = chat.SetAttribute(to, ContactList, to_contacts)
		if err != nil {
			return err
		}
	}
	return nil
}
