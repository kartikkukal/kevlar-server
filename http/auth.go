package http

import (
	"encoding/json"
	"errors"
	"kevlar/module/attr"
	"kevlar/module/auth"
	"kevlar/module/sec"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"
)

type API struct {
	auth.Auth
}

func loadBody(request *http.Request, v any) error {
	err := json.NewDecoder(request.Body).Decode(v)
	if err != nil {
		return err
	}
	return nil
}
func setCookie(userID, session, domain string, response http.ResponseWriter) {
	data := struct {
		UserID  string `json:"userID"`
		Session string `json:"session"`
	}{
		UserID:  userID,
		Session: session,
	}
	dataJson, _ := json.Marshal(data)
	encoded := sec.EncodeBase64(dataJson)
	sessionCookie := http.Cookie{
		Name:     "session",
		Value:    encoded,
		Domain:   domain,
		Path:     "/",
		Expires:  time.Now().Add(auth.SessionExpiry),
		Secure:   false,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(response, &sessionCookie)
}

func (main Server) AccountHandle() http.HandlerFunc {
	type Request struct {
		UserID   string `json:"userID"`
		Username string `json:"username"`
		Password string `json:"password"`
		Relogin  string `json:"relogin"`
	}
	type Response struct {
		Relogin string `json:"relogin"`
	}

	log := logrus.WithField("method", "accountHandler")

	return func(response http.ResponseWriter, request *http.Request) {

		defer request.Body.Close()

		// Register error handler
		handler := errorHandler(response, request, log)

		// Get router arguements
		args := mux.Vars(request)
		action, ok := args["action"]
		if !ok {
			handler(errors.New("action not present"), 404, "action not present")
			return
		}

		// Get request data
		var data Request
		err := loadBody(request, &data)

		if err != nil {
			handler(err, 400, "error while parsing body")
			return
		}

		var relogin string

		// Method handlers
		if action == "create" {
			relogin, err = main.auth.Create(data.UserID, data.Username, data.Password)

			if err != nil {
				if mongo.IsDuplicateKeyError(err) {
					handler(err, 409, "error while creating account")
					return
				}
				if errors.Is(err, auth.ErrInvalidCharacter) {
					handler(err, 422, "error while creating account")
					return
				}
				if errors.Is(err, auth.ErrIncorrectInputLength) {
					handler(err, 413, "error while creating account")
					return
				}
				handler(err, 400, "error while creating account")
				return
			}

			err = main.EventCreate(data.UserID)
			if err != nil {
				handler(err, 400, "create account event error")

				// Delete account entry if there is an error in the EventCreate method.
				err = main.auth.Delete(data.UserID, data.Password)
				if err != nil {
					handler(err, 400, "error while removing newly created account")
				}
				err = main.EventDelete(data.UserID)
				if err != nil {
					handler(err, 400, "error while removing newly created account")
				}
				return
			}

			log.WithField("userID", data.UserID).Info("account created")

		} else if action == "login" {
			relogin, err = main.auth.Login(data.UserID, data.Password)

			if err != nil {
				handler(err, 400, "error while logging in")
				return
			}

			log.WithField("userID", data.UserID).Info("account logged in")

		} else if action == "logout" {
			userID, err := main.authenticate(request)
			if err != nil {
				if err == attr.ErrSessionExpired {
					handler(err, 401, "error while verifying session")
					return
				}
				handler(err, 400, "error while authenticating user")
				return
			}

			err = main.auth.Logout(userID, data.Relogin)

			if err != nil {
				handler(err, 400, "error while logging out")
				return
			}

			err = main.EventLogout(userID)
			if err != nil {
				handler(err, 400, "logout event error")
				return
			}

			log.WithField("userID", data.UserID).Info("account logged out")
			return

		} else if action == "relogin" {
			data.UserID, relogin, err = main.auth.Autologin(data.Relogin)

			if err != nil {
				handler(err, 400, "error while relogging account")
				return
			}

			log.Info("account relogged")

		} else if action == "delete" {
			err = main.auth.Delete(data.UserID, data.Password)
			if err != nil {
				handler(err, 400, "error while deleting account")
				return
			}

			err = main.EventDelete(data.UserID)
			if err != nil {
				handler(err, 400, "error while deleting account")
				return
			}

			log.WithField("userID", data.UserID).Info("account deleted")

			response.WriteHeader(201)
			return
		} else {
			main.NotFoundHandler.ServeHTTP(response, request)
			return
		}

		session, err := main.attr.CreateSession(data.UserID)
		if err != nil {
			handler(err, 400, "error while generating session")
			return
		}

		setCookie(data.UserID, session, main.auth.Domain, response)

		responseData, err := json.Marshal(
			Response{
				Relogin: relogin,
			})
		if err != nil {
			handler(err, 400, "error while marshalling response")
			return
		}

		response.Write(responseData)
	}
}

func (main Server) VerifySession() http.HandlerFunc {

	type Request struct {
		Session string `json:"session"`
	}

	log := logrus.WithField("method", "verifySession")

	return func(response http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()

		// Register handlers
		handler := errorHandler(response, request, log)

		var requestData Request
		err := loadBody(request, &requestData)

		if err != nil {
			handler(err, 400, "error while parsing body")
			return
		}

		// Authenticate user
		value, err := sec.DecodeBase64(requestData.Session)
		if err != nil {
			handler(err, 400, "error while parsing session")
			return
		}

		var data Cookie

		err = json.Unmarshal([]byte(value), &data)
		if err != nil {
			handler(err, 400, "error while decoding session")
			return
		}

		err = main.attr.VerifySession(data.UserID, data.Session)
		if err != nil {
			handler(err, 400, "error while verifying session")
			return
		}

		response.WriteHeader(200)
	}
}

func (main Server) InputLimits() http.HandlerFunc {

	type Response struct {
		InputMin int `json:"input_minimum"`
		InputMax int `json:"input_maximum"`
	}

	log := logrus.WithField("method", "verifySession")

	return func(response http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()

		// Register handlers
		handler := errorHandler(response, request, log)

		responseData, err := json.Marshal(Response{
			InputMin: main.auth.MinInputLength,
			InputMax: main.auth.MaxInputLength,
		})
		if err != nil {
			handler(err, 400, "error while marshalling response")
			return
		}

		response.Write(responseData)
	}
}
func (main Server) RegisterAuthenticationHandlers() {
	main.HandleFunc("/auth/limits", main.InputLimits()).Methods("POST", "OPTIONS")

	main.HandleFunc("/auth/{action}", main.AccountHandle()).Methods("POST", "OPTIONS")       // for preflight
	main.HandleFunc("/auth/session/verify", main.VerifySession()).Methods("POST", "OPTIONS") // for preflight
}
