package http

import (
	"encoding/json"
	"errors"
	"kevlar/module/attr"
	"kevlar/module/chat"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

var (
	RequestSent           = "request_sent"
	RequestReceived       = "request_recieved"
	RequestCancelIncoming = "request_cancel_incoming"
	RequestCancelOutgoing = "request_cancel_outgoing"
	RequestDecideIncoming = "request_decide_incoming"
	RequestDecideOutgoing = "request_decide_outgoing"
	OnlineStatus          = "user_online_update"
)

func (main Server) GetAttributes() http.HandlerFunc {

	log := logrus.WithField("method", "getChatAttributes")

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

		responseData, err := main.chat.GetAttributes(userID, main.IsOnline)
		if err != nil {
			handler(err, 400, "error while getting attributes")
			return
		}

		data, err := json.Marshal(responseData)
		if err != nil {
			handler(err, 400, "error while marshalling response")
			return
		}

		response.WriteHeader(200)
		response.Write(data)
	}
}

func (main Server) About() http.HandlerFunc {

	type Request struct {
		About string `json:"about"`
	}

	log := logrus.WithField("method", "setAbout")

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

		// Get request data
		var requestData Request
		err = loadBody(request, &requestData)

		if err != nil {
			handler(err, 400, "error while parsing body")
			return
		}

		err = main.attr.SetAttribute(userID, chat.About, requestData.About)
		if err != nil {
			handler(err, 400, "error while setting about")
			return
		}

		response.WriteHeader(200)
	}
}

func (main Server) Search() http.HandlerFunc {
	type Request struct {
		Search string `json:"query"`
		LastID string `json:"last_id"`
	}
	type Response struct {
		List   []chat.User `json:"result"`
		LastID string      `json:"last_id"`
	}

	log := logrus.WithField("method", "searchUsers")

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

		// Get request data
		var requestData Request
		err = loadBody(request, &requestData)

		if err != nil {
			handler(err, 400, "error while parsing body")
			return
		}

		list, last_id, err := main.chat.Search(userID, requestData.Search, requestData.LastID)
		if err != nil {
			handler(err, 400, "error while searching")
			return
		}

		responseData := Response{
			List:   list,
			LastID: last_id,
		}

		data, err := json.Marshal(responseData)
		if err != nil {
			handler(err, 400, "error while marshalling response")
			return
		}

		response.WriteHeader(200)
		response.Write(data)
	}
}

func (main Server) Request() http.HandlerFunc {
	type Request struct {
		UserID string `json:"userID"`
		Decide bool   `json:"decide,omitempty"`
	}

	log := logrus.WithField("method", "sendRequest")

	return func(response http.ResponseWriter, request *http.Request) {

		defer request.Body.Close()

		// Register handlers
		handler := errorHandler(response, request, log)

		// Get vars
		args := mux.Vars(request)
		action, ok := args["action"]
		if !ok {
			handler(errors.New("action not present"), 404, "action not present")
			return
		}

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

		// Get request data
		var requestData Request
		err = loadBody(request, &requestData)

		if err != nil {
			handler(err, 400, "error while parsing body")
			return
		}

		fromUser, err := main.chat.GetInformation(userID)
		if err != nil {
			handler(err, 400, "error while getting user attributes")
			return
		}

		toUser, err := main.chat.GetInformation(requestData.UserID)
		if err != nil {
			handler(err, 400, "error while getting user attributes")
			return
		}

		if action == "send" {
			err = main.chat.Request(userID, requestData.UserID)

			if err != nil {
				if errors.Is(err, chat.ErrRequestSent) {
					handler(err, 409, "error while adding request")
					return
				}
				handler(err, 400, "error while adding request")
				return
			}

			// Notify the receiver that request has been sent.
			main.WriteMessage(requestData.UserID, struct {
				Head string      `json:"head"`
				Data interface{} `json:"data"`
			}{
				Head: RequestReceived,
				Data: struct {
					User chat.User `json:"user"`
				}{
					User: fromUser,
				},
			})

			// Notify the sender that request has been sent.
			main.WriteMessage(userID, struct {
				Head string      `json:"head"`
				Data interface{} `json:"data"`
			}{
				Head: RequestSent,
				Data: struct {
					User chat.User `json:"user"`
				}{
					User: toUser,
				},
			})

		} else if action == "cancel" {
			err = main.chat.Cancel(userID, requestData.UserID)

			if err != nil {
				handler(err, 400, "error while cancelling request")
				return
			}

			// Notify the receiver that request has been cancelled.
			main.WriteMessage(requestData.UserID, struct {
				Head string      `json:"head"`
				Data interface{} `json:"data"`
			}{
				Head: RequestCancelIncoming,
				Data: struct {
					User chat.User `json:"user"`
				}{
					User: fromUser,
				},
			})

			// Notify the sender that request has been cancelled.
			main.WriteMessage(userID, struct {
				Head string      `json:"head"`
				Data interface{} `json:"data"`
			}{
				Head: RequestCancelOutgoing,
				Data: struct {
					User chat.User `json:"user"`
				}{
					User: toUser,
				},
			})

		} else if action == "decide" {
			err = main.chat.Decide(userID, requestData.UserID, requestData.Decide)

			if err != nil {
				handler(err, 400, "error while deciding request")
				return
			}

			if requestData.Decide {
				// Notify the receiver that request has been decided.
				main.WriteMessage(requestData.UserID, struct {
					Head string      `json:"head"`
					Data interface{} `json:"data"`
				}{
					Head: RequestDecideOutgoing,
					Data: struct {
						User     interface{} `json:"user"`
						Decision bool        `json:"decision"`
					}{
						User: struct {
							chat.User
							Online bool `json:"online"`
						}{
							User:   fromUser,
							Online: main.IsOnline(userID),
						},
						Decision: requestData.Decide,
					},
				})

				// Notify the sender that request has been decided.
				main.WriteMessage(userID, struct {
					Head string      `json:"head"`
					Data interface{} `json:"data"`
				}{
					Head: RequestDecideIncoming,
					Data: struct {
						User     interface{} `json:"user"`
						Decision bool        `json:"decision"`
					}{
						User: struct {
							chat.User
							Online bool `json:"online"`
						}{
							User:   toUser,
							Online: main.IsOnline(requestData.UserID),
						},
						Decision: requestData.Decide,
					},
				})

			} else {
				main.WriteMessage(requestData.UserID, struct {
					Head string      `json:"head"`
					Data interface{} `json:"data"`
				}{
					Head: RequestDecideOutgoing,
					Data: struct {
						User     chat.User `json:"user"`
						Decision bool      `json:"decision"`
					}{
						User:     fromUser,
						Decision: requestData.Decide,
					},
				})

				// Notify the sender that request has been decided.
				main.WriteMessage(userID, struct {
					Head string      `json:"head"`
					Data interface{} `json:"data"`
				}{
					Head: RequestDecideIncoming,
					Data: struct {
						User     chat.User `json:"user"`
						Decision bool      `json:"decision"`
					}{
						User:     toUser,
						Decision: requestData.Decide,
					},
				})
			}

		}

		response.WriteHeader(201)
	}
}

func (main Server) Message() http.HandlerFunc {
	type Request struct {
		Type string `json:"type"`
		Data string `json:"data"`
	}

	type Response struct {
		Read bool `json:"read"`
	}

	log := logrus.WithField("method", "sendMessage")

	return func(response http.ResponseWriter, request *http.Request) {

		defer request.Body.Close()

		// Register handlers
		handler := errorHandler(response, request, log)

		args := mux.Vars(request)
		toUserID, ok := args["userID"]
		if !ok {
			handler(errors.New("userID not present"), 404, "userID not present")
			return
		}

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

		// Get request data
		var requestData Request
		err = loadBody(request, &requestData)

		if err != nil {
			handler(err, 400, "error while parsing body")
			return
		}

		_, online := main.socket[toUserID]

		err = main.chat.StoreMessage(requestData.Type, requestData.Data, userID, toUserID, online)
		if err != nil {
			handler(err, 400, "error while storing message")
			return
		}

		main.WriteMessage(toUserID, struct {
			Head string      `json:"head"`
			Data interface{} `json:"data"`
		}{
			Head: chat.MessageIncoming,
			Data: struct {
				From string `json:"from"`
				Type string `json:"type"`
				Data string `json:"data"`
			}{
				From: userID,
				Type: requestData.Type,
				Data: requestData.Data,
			},
		})

		responseData := Response{
			Read: online,
		}

		data, err := json.Marshal(responseData)
		if err != nil {
			handler(err, 400, "error while marshalling response")
			return
		}

		response.WriteHeader(200)
		response.Write(data)
	}
}

func (main Server) LoadPrevious() http.HandlerFunc {
	type Request struct {
		Since int `json:"since"`
	}

	type Response struct {
		Messages []chat.Message `json:"messages"`
	}

	log := logrus.WithField("method", "sendMessage")

	return func(response http.ResponseWriter, request *http.Request) {

		defer request.Body.Close()

		// Register handlers
		handler := errorHandler(response, request, log)

		args := mux.Vars(request)
		toUserID, ok := args["userID"]
		if !ok {
			handler(errors.New("userID not present"), 404, "userID not present")
			return
		}

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

		// Get request data
		var requestData Request
		err = loadBody(request, &requestData)

		if err != nil {
			handler(err, 400, "error while parsing body")
			return
		}

		messages, err := main.chat.LoadMessages(userID, toUserID, requestData.Since)
		if err != nil {
			handler(err, 400, "error while storing message")
			return
		}

		responseData := Response{
			Messages: messages,
		}

		data, err := json.Marshal(responseData)
		if err != nil {
			handler(err, 400, "error while marshalling response")
			return
		}

		response.WriteHeader(200)
		response.Write(data)
	}
}

func (main Server) RegisterChatAPIHandlers() {
	main.HandleFunc("/chat/data/all", main.GetAttributes()).Methods("POST", "OPTIONS")
	main.HandleFunc("/chat/data/about", main.About()).Methods("POST", "OPTIONS")
	main.HandleFunc("/chat/search", main.Search()).Methods("POST", "OPTIONS")
	main.HandleFunc("/chat/request/{action}", main.Request()).Methods("POST", "OPTIONS")
	main.HandleFunc("/chat/message/{userID}", main.Message()).Methods("POST", "OPTIONS")
	main.HandleFunc("/chat/history/{userID}", main.LoadPrevious()).Methods("POST", "OPTIONS")
}
