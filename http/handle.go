package http

import (
	"encoding/json"
	"errors"
	"kevlar/module/chat"

	"github.com/sirupsen/logrus"
)

type SocketHandler func(userID string, data json.RawMessage) (interface{}, error)

type IncomingMessage struct {
	Head string          `json:"head"`
	Data json.RawMessage `json:"data"`
}

const (
	TypingStatusUpdate = "typing_status_update"
)

var (
	ErrSocketDoesNotExist = errors.New("the specified socket does not exist or has closed")
	Methods               = make(map[string]SocketHandler)
)

func (main Server) TypingStatusUpdate() SocketHandler {

	type Request struct {
		UserID string `json:"userID"`
		Typing bool   `json:"typing"`
	}

	return func(userID string, data json.RawMessage) (interface{}, error) {
		var request Request
		err := json.Unmarshal(data, &request)
		if err != nil {
			return nil, err
		}

		var contacts []chat.User
		err = main.attr.GetAttribute(userID, chat.ContactList, &contacts)
		if err != nil {
			return nil, err
		}

		typing := func() {
			main.WriteMessage(request.UserID, struct {
				Head string      `json:"head"`
				Data interface{} `json:"data"`
			}{
				Head: TypingStatusUpdate,
				Data: struct {
					UserID string `json:"userID"`
					Typing bool   `json:"typing"`
				}{
					UserID: userID,
					Typing: request.Typing,
				},
			})
		}

		for _, value := range contacts {
			if value.UserID == request.UserID {
				typing()
			}
		}

		return nil, nil
	}
}

func ConnectionHandle(socket *Socket, userID string) {

	log := logrus.WithField("userID", userID).WithField("method", "websocketHandler")

	for {
		data, ok := <-socket.Recieve
		if !ok {
			log.Trace("incoming channel exited")
			break
		}

		var packet IncomingMessage
		err := json.Unmarshal(data, &packet)
		if err != nil {
			log.Error("error while unmarshalling message in websocket")
			return
		}

		for head, handler := range Methods {
			if head == packet.Head {
				response, err := handler(userID, packet.Data)
				if err != nil {
					log.WithError(err).Error("error while executing websocket method")
				}

				if response != nil {
					socket.Send <- response
				}

				break
			}
		}
	}
	log.Trace("handler exit")
}

func (main Server) WebsocketConnectionHandler(userID string) error {
	socket, ok := main.socket[userID]
	if !ok {
		return ErrSocketDoesNotExist
	}

	main.RegisterWebsocketMethods()

	go ConnectionHandle(socket, userID)

	return nil
}

func register(head string, handler SocketHandler) {
	Methods[head] = handler
}

func (main Server) RegisterWebsocketMethods() {
	register(TypingStatusUpdate, main.TypingStatusUpdate())
}
