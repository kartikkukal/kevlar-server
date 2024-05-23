package http

import (
	"fmt"
	"kevlar/module/chat"
	"kevlar/module/file"
	"kevlar/module/img"
	"time"
)

func (main Server) EventCreate(userID string) error {
	// function is called when a user is registered.

	err := main.attr.CreateUser(userID)
	if err != nil {
		return err
	}

	err = main.store.CreateUser(userID)
	if err != nil {
		return err
	}

	err = main.chat.CreateUser(userID)
	if err != nil {
		return err
	}

	err = func() error {

		user, err := main.chat.GetInformation(userID)
		if err != nil {
			return err
		}

		profile, err := img.GenerateImage(user.Username)
		if err != nil {
			return err
		}

		image := file.New(profile, "profile.png", []string{})

		image.Attributes.ID = "profile"

		err = main.store.Upload(userID, &image)

		return err
	}()
	if err != nil {
		return err
	}

	return nil
}
func (main Server) EventLogout(userID string) error {
	// function is called when a user is logged out.

	err := main.attr.DeleteSession(userID)
	if err != nil {
		return err
	}

	return nil
}
func (main Server) EventDelete(userID string) error {
	// function is called when a user isdeleted

	err := main.chat.DeleteUser(userID)
	if err != nil {
		return fmt.Errorf("error while deleting chat data: %w", err)
	}

	err = main.store.DeleteAll(userID)
	if err != nil {
		return fmt.Errorf("error while deleting storage data: %w", err)
	}

	err = main.attr.DeleteUser(userID)
	if err != nil {
		return fmt.Errorf("error while deleting attributes: %w", err)
	}

	return nil
}

func (main Server) WebSocketConnected(userID string) error {

	var contacts []chat.User

	err := main.attr.GetAttribute(userID, chat.ContactList, &contacts)
	if err != nil {
		return err
	}

	for _, contact := range contacts {
		main.WriteMessage(contact.UserID, struct {
			Head string      `json:"head"`
			Data interface{} `json:"data"`
		}{
			Head: OnlineStatus,
			Data: struct {
				UserID string `json:"userID"`
				Online bool   `json:"online"`
			}{
				UserID: userID,
				Online: true,
			},
		})
	}

	err = main.WebsocketConnectionHandler(userID)

	return err
}

func (main Server) WebSocketDisconnected(userID string) error {

	var contacts []chat.User

	err := main.attr.GetAttribute(userID, chat.ContactList, &contacts)
	if err != nil {
		return err
	}

	for _, contact := range contacts {
		main.WriteMessage(contact.UserID, struct {
			Head string      `json:"head"`
			Data interface{} `json:"data"`
		}{
			Head: OnlineStatus,
			Data: struct {
				UserID string `json:"userID"`
				Online bool   `json:"online"`
			}{
				UserID: userID,
				Online: false,
			},
		})
	}

	err = main.attr.SetAttribute(userID, chat.LastSeen, time.Now().Unix())

	return err
}
