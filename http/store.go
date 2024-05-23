package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"kevlar/module/attr"
	"kevlar/module/file"
	"kevlar/module/img"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

type Cookie struct {
	Session string `json:"session"`
	UserID  string `json:"userID"`
}

func ETag(length int, fileID string) string {
	return fmt.Sprintf("W/%x-%x", length, crc32.ChecksumIEEE([]byte(fileID)))
}

func (main Server) Upload() http.HandlerFunc {

	type Response struct {
		FileID string `json:"fileID"`
	}

	log := logrus.WithField("method", "Upload")

	return func(response http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()

		// Register handler
		handler := errorHandler(response, request, log)

		// Authenticate user
		userID, err := main.authenticate(request)
		if err != nil {
			if errors.Is(err, attr.ErrSessionExpired) {
				handler(err, 401, "error while verifying session")
				return
			}
			handler(err, 400, "error while authenticating user")
			return
		}

		// Limit max body size
		request.Body = http.MaxBytesReader(response, request.Body, main.store.MaxUploadLimitMB<<20)

		// Parse form
		err = request.ParseMultipartForm(main.store.MaxUploadLimitMB << 20)

		if err != nil {
			handler(err, 400, "error while parsing form")
			return
		}

		header, ok := request.MultipartForm.File["data"]
		if !ok {
			handler(nil, 400, "required feild absent in form")
			return
		}

		perm, ok := request.MultipartForm.Value["perm"]
		if !ok {
			handler(err, 400, "required feild absent in form")
			return
		}

		// Check if there is feild
		if len(perm) != 1 {
			handler(nil, 400, "feild invalid")
			return
		}

		if len(header) != 1 {
			handler(nil, 400, "invalid number of files")
			return
		}

		reader, err := header[0].Open()
		if err != nil {
			handler(err, 400, "unable to open file")
			return
		}

		data, err := io.ReadAll(reader)
		if err != nil {
			handler(err, 400, "unable to read file")
			return
		}

		// Convert slashes and strip leading path.
		name := filepath.Base(strings.ReplaceAll(header[0].Filename, "\\", "/"))

		file := file.New(data, name, perm)

		err = main.store.Upload(userID, &file)
		if err != nil {
			handler(err, 400, "error while uploading file")
			return
		}

		result, err := json.Marshal(Response{
			FileID: file.Attributes.ID,
		})
		if err != nil {
			handler(err, 400, "error while marshalling response")
			return
		}

		log.Info("file uploaded")

		response.WriteHeader(200)
		response.Write(result)
	}
}

func (main Server) Download() http.HandlerFunc {

	log := logrus.WithField("method", "Download")
	return func(response http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()

		args := mux.Vars(request)

		// Register handler
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

		fromUserID, ok := args["userID"]
		if !ok {
			fromUserID = userID
		}

		fileID, ok := args["fileID"]
		if !ok {
			handler(err, 404, "not found")
			return
		}

		file, err := main.store.Download(userID, fromUserID, fileID)
		if err != nil {
			handler(err, 400, "error while downloading file")
			return
		}

		etag := ETag(len(file.Data), fileID)

		if strings.TrimSpace(request.Header.Get("If-None-Match")) == etag {
			response.WriteHeader(304)
			response.Write(nil)
			return
		}

		reader := bytes.NewReader(file.Data)

		response.Header().Add("Cache-Control", "max-age:0")
		response.Header().Add("ETag", etag)

		http.ServeContent(response, request, file.Attributes.Name, time.Time{}, reader)
	}
}

func (main Server) Delete() http.HandlerFunc {

	log := logrus.WithField("method", "Delete")
	return func(response http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()

		args := mux.Vars(request)

		// Register handler
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

		fileID, ok := args["fileID"]
		if !ok {
			handler(err, 404, "not found")
			return
		}

		err = main.store.DeleteOne(userID, fileID)
		if err != nil {
			handler(err, 500, "error while deleting file")
		}

		response.WriteHeader(201)
	}
}

func (main Server) List() http.HandlerFunc {

	log := logrus.WithField("method", "List")
	return func(response http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()

		// Register handler
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

		files, err := main.store.ListAll(userID)
		if err != nil {
			handler(err, 400, "error while getting file list")
		}

		data, err := json.Marshal(files)
		if err != nil {
			handler(err, 400, "error while marshalling response")
			return
		}

		response.WriteHeader(200)
		response.Write(data)
	}
}

func (main Server) Attributes() http.HandlerFunc {
	type Request struct {
		Name string   `json:"name"`
		Perm []string `json:"perm"`
		Add  []string `json:"add_perm"`
	}

	log := logrus.WithField("method", "Attributes")
	return func(response http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()

		// Register handler
		handler := errorHandler(response, request, log)

		args := mux.Vars(request)

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

		var requestData Request

		fileID, ok := args["fileID"]
		if !ok {
			handler(err, 404, "not found")
			return
		}

		err = loadBody(request, &requestData)
		if err != nil {
			handler(err, 400, "error while parsing body")
			return
		}

		update_name := (requestData.Name != "")
		update_perm := (requestData.Perm != nil)
		add_perm := (requestData.Add != nil)

		attributes, err := main.store.GetFileAttributes(userID, fileID)
		if err != nil {
			handler(err, 400, "error while getting file attributes")
			return
		}

		if update_name {
			attributes.Name = requestData.Name
		}
		if update_perm {
			attributes.Perm = requestData.Perm
		} else if add_perm {
			attributes.Perm = append(attributes.Perm, requestData.Add...)
		}

		err = main.store.SetFileAttributes(userID, fileID, attributes)
		if err != nil {
			handler(err, 400, "error while setting file attributes")
			return
		}

		response.WriteHeader(201)
	}
}

func (main Server) Profile() http.HandlerFunc {

	log := logrus.WithField("method", "Profile")
	return func(response http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()

		// Register handler
		handler := errorHandler(response, request, log)

		// Authenticate user
		userID, err := main.authenticate(request)
		if err != nil {
			if errors.Is(err, attr.ErrSessionExpired) {
				handler(err, 401, "error while verifying session")
				return
			}
			handler(err, 400, "error while authenticating user")
			return
		}

		// Parse form
		err = request.ParseMultipartForm(2 << 20)

		if err != nil {
			handler(err, 400, "error while parsing form")
			return
		}

		header, ok := request.MultipartForm.File["data"]
		if !ok {
			handler(nil, 400, "required feild absent in form")
			return
		}

		// Check if there is feild
		if len(header) != 1 {
			handler(nil, 400, "invalid number of files")
			return
		}

		reader, err := header[0].Open()
		if err != nil {
			handler(err, 400, "unable to open file")
			return
		}

		data, err := io.ReadAll(reader)
		if err != nil {
			handler(err, 400, "unable to read file")
			return
		}

		// Convert slashes and strip leading path.

		err = main.store.DeleteOne(userID, "profile")
		if err != nil {
			handler(err, 400, "error while removing profile")
			return
		}

		// Resize image
		resized, err := img.ResizeImage(data)
		if err != nil {
			handler(err, 400, "error while resizing image")
			return
		}

		file := file.New(resized, "profile.png", []string{})

		file.Attributes.ID = "profile"

		err = main.store.Upload(userID, &file)
		if err != nil {
			handler(err, 400, "error while uploading profile")
			return
		}

		log.Info("file uploaded")

		response.WriteHeader(200)
	}
}

func (main Server) RegisterStorageHandlers() {
	main.HandleFunc("/store/upload", main.Upload()).Methods("POST", "OPTIONS")
	main.HandleFunc("/store/{fileID}", main.Download()).Methods("GET", "OPTIONS")
	main.HandleFunc("/store/{userID}/{fileID}", main.Download()).Methods("GET", "OPTIONS")
	main.HandleFunc("/store/{fileID}", main.Delete()).Methods("DELETE", "OPTIONS")
	main.HandleFunc("/store/list", main.List()).Methods("POST", "OPTIONS")
	main.HandleFunc("/store/update/{fileID}", main.Attributes()).Methods("POST", "OPTIONS")
	main.HandleFunc("/store/profile", main.Profile()).Methods("POST", "OPTIONS")
}
