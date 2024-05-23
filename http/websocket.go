package http

import (
	"errors"
	"kevlar/module/attr"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

type Socket struct {
	Wait *sync.WaitGroup

	Send    chan interface{}
	Recieve chan []byte

	Close chan bool
}

var (
	upgrader      = websocket.Upgrader{}
	readDeadline  = 30 * time.Second
	writeDeadline = 30 * time.Second

	pingDuration = 20 * time.Second

	ErrChannelClosed = errors.New("channel is closed")
)

// Handler for realtime websocket connections.
func (main Server) WebSocketHandler() http.HandlerFunc {
	log := logrus.WithField("method", "websocket")

	return func(response http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()

		// Register handlers
		handler := errorHandler(response, request, log)

		// Authenticate user
		userID, err := main.authenticate(request)
		if err != nil {
			if err == attr.ErrSessionExpired {
				handler(err, 401, "error while verifying session")
				return
			}
			handler(err, 400, "error while authenticating user")
			return
		}

		// Close previous connection
		old, ok := main.socket[userID]
		if ok {
			old.Close <- true

			log.WithField("userID", userID).Trace("closing existing websocket connection")
			old.Wait.Wait()
		}

		// Upgrade to websocket
		conn, err := upgrader.Upgrade(response, request, nil)
		if err != nil {
			log.WithField("userID", userID).WithError(err).Error("error while upgrading to websocket")
			return
		}

		log.Trace("websocket connection established: ", userID)

		socket := Socket{
			Wait: &sync.WaitGroup{},

			Send:    make(chan interface{}, 10),
			Recieve: make(chan []byte, 10),

			Close: make(chan bool, 10),
		}

		main.socket[userID] = &socket

		// Websocket connection event
		err = main.WebSocketConnected(userID)
		if err != nil {
			log.WithField("userID", userID).WithError(err).Error("error while calling websocket connected event")
			return
		}

		// Websocket reader
		socket.Wait.Add(1)

		go func() {
			for {
				conn.SetReadDeadline(time.Now().Add(readDeadline))

				var data []byte
				_, data, err := conn.ReadMessage()
				if err != nil {
					socket.Close <- true

					log.WithField("userID", userID).WithError(err).Error("error while receiving message in websocket")
					break
				}

				socket.Recieve <- data
			}

			socket.Wait.Done()
		}()

		// Websocket sender
		ping := time.NewTicker(pingDuration)
		socket.Wait.Add(1)

		func() {
			for {
				select {
				case message, ok := <-socket.Send:
					if !ok {
						socket.Close <- true
						return
					}

					conn.SetWriteDeadline(time.Now().Add(writeDeadline))

					err = conn.WriteJSON(message)
					if err != nil {
						socket.Close <- true

						log.WithField("userID", userID).WithError(err).Error("error while writing message to websocket")
						return
					}

				case <-ping.C:
					socket.Send <- struct {
						Head string `json:"head"`
					}{
						Head: "ping",
					}

				case <-socket.Close:
					return
				}
			}
		}()

		socket.Wait.Done()

		err = conn.Close()
		if err != nil {
			log.WithField("userID", userID).WithError(err).Error("error while closing websocket")
			return
		}

		err = main.WebSocketDisconnected(userID)
		if err != nil {
			log.WithField("userID", userID).WithError(err).Error("error while calling websocket disconnected event")
			return
		}

		defer func() {
			socket.Wait.Wait()
			delete(main.socket, userID)

			logrus.WithField("userID", userID).Trace("connection closed")
		}()
	}
}

// Try to write the message to websocket if user is online, otherwise throw error.
func (main Server) WriteMessage(userID string, data interface{}) error {
	sender, ok := main.socket[userID]
	if !ok {
		return ErrChannelClosed
	}

	sender.Send <- data

	return nil
}

// Check if user is online (connected to websocket)
func (main Server) IsOnline(userID string) bool {
	_, ok := main.socket[userID]
	return ok
}

func (main Server) RegisterWebsocketHandler(domainName string) {
	upgrader.CheckOrigin = func(request *http.Request) bool {
		return request.Header.Get("Origin") == domainName
	}
	main.HandleFunc("/ws", main.WebSocketHandler()).Methods("GET", "OPTIONS")
}
