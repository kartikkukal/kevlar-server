package http

import (
	"context"
	"encoding/json"
	"kevlar/module/attr"
	"kevlar/module/auth"
	"kevlar/module/chat"
	"kevlar/module/conf"
	"kevlar/module/db/minio"
	"kevlar/module/db/mongo"
	"kevlar/module/sec"
	"kevlar/module/store"

	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

func log(request *http.Request) {
	logrus.WithFields(logrus.Fields{
		"method": request.Method,
		"path":   request.RequestURI,
		"ua":     request.UserAgent(),
		"ref":    request.Referer(),
		"addr":   request.RemoteAddr,
	}).Trace("http request")
}

// Logger middleware
func loggerMiddleware(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		log(request)
		handler.ServeHTTP(response, request)
	})
}

// CORS middleware
func corsMiddleware(domainName string) mux.MiddlewareFunc {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			response.Header().Add("Access-Control-Allow-Credentials", "true")
			response.Header().Add("Access-Control-Allow-Origin", domainName)
			if request.Method == "OPTIONS" {
				response.Header().Add("Access-Control-Allow-Headers", "Content-Type")
				response.Header().Add("Access-Control-Allow-Methods", "POST")
				response.Write(nil)
				return
			}
			handler.ServeHTTP(response, request)
		})
	}
}

func errorHandler(response http.ResponseWriter, request *http.Request, log *logrus.Entry) func(err error, code int, msg string) {
	return func(err error, code int, msg string) {
		log.WithFields(logrus.Fields{
			"path": request.URL.Path,
			"code": code,
		}).WithError(err).Error(msg)
		response.WriteHeader(code)
		response.Write(nil)
	}
}

// Not found handler
type notFound struct{}

func (n notFound) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	log(request)
	response.WriteHeader(404)
}

type Server struct {
	http.Server
	*mux.Router
	attr   attr.Attr
	auth   auth.Auth
	store  store.Store
	chat   chat.Chat
	config conf.RootConfig
	socket map[string]*Socket
}

func New(mongo *mongo.MongoClient, minio *minio.MinioClient, config conf.RootConfig) Server {
	router := mux.NewRouter()
	router.NotFoundHandler = notFound{}
	router.Use(loggerMiddleware)
	router.Use(corsMiddleware(config.Http.DomainName))

	attr := attr.New(mongo, config.Attr)
	auth := auth.New(mongo, config.Auth)
	store := store.New(minio, attr, config.Store)
	chat := chat.New(attr, mongo)

	sockets := make(map[string]*Socket)

	server := Server{
		Router: router,
		attr:   attr,
		auth:   auth,
		store:  store,
		chat:   chat,
		config: config,
		socket: sockets,
	}
	server.Server = http.Server{
		Addr:    server.config.Http.Address,
		Handler: server.Router,
	}
	server.registerAll()
	return server
}

func (server Server) Ping() http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		response.Write([]byte("ping"))
	}
}

func (main Server) authenticate(request *http.Request) (string, error) {
	cookie, err := request.Cookie("session")
	if err != nil {
		return "", err
	}
	encoded := cookie.Value

	value, err := sec.DecodeBase64(encoded)
	if err != nil {
		return "", err
	}

	var data Cookie

	err = json.Unmarshal([]byte(value), &data)
	if err != nil {
		return "", err
	}

	err = main.attr.VerifySession(data.UserID, data.Session)
	if err != nil {
		return "", err
	}
	return data.UserID, nil
}

func (main Server) registerAll() {

	main.HandleFunc("/ping", main.Ping())

	main.RegisterAuthenticationHandlers()

	main.RegisterStorageHandlers()

	main.RegisterChatAPIHandlers()

	main.RegisterWebsocketHandler(main.config.Http.DomainName)
}

func (server Server) Start() {
	logrus.Trace("started http server")
	err := server.ListenAndServe()
	if err != nil {
		logrus.WithError(err).Error("server error")
	}
}
func (server Server) Stop() {
	context, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	server.Shutdown(context)
	logrus.Trace("http server closed")
}
