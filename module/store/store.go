package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"kevlar/module/attr"
	miniodb "kevlar/module/db/minio"
	"kevlar/module/file"
	"math"
	"strings"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/sirupsen/logrus"
)

var (
	ErrQuotaFull        = errors.New("account quota is full")
	ErrUserNotAllowed   = errors.New("user cannot access this file")
	ErrBucketNotPresent = errors.New("bucket does not exist")
	ErrInvalidCast      = errors.New("invalid cast to type")
	quota_used          = "quota_used"
)

type Config struct {
	MaxUploadLimitMB int64   `default:"128"`
	QuotaLimitMB     float64 `default:"256"`
}

type Store struct {
	*miniodb.MinioClient
	attr.Attr
	Config
}

func New(minio *miniodb.MinioClient, attr attr.Attr, config Config) Store {
	return Store{
		MinioClient: minio,
		Attr:        attr,
		Config:      config,
	}
}

func sizeMB(bytes int) float64 {
	return (math.Ceil((float64(bytes)/1024.0/1024.0)*100) / 100)
}

// Creates required attributes for storage system.
func (store Store) CreateUser(userID string) error {

	context, cancel := store.DefaultContext()
	defer cancel()

	bucketID := uuid.New().String()

	err := store.SetAttribute(userID, "bucket", bucketID)
	if err != nil {
		return err
	}

	err = store.SetAttribute(userID, "quota_used", float64(0.0))
	if err != nil {
		return err
	}

	err = store.MakeBucket(context, bucketID, minio.MakeBucketOptions{})
	if err != nil {
		return err
	}

	return nil
}

// PutFile:
// Puts the file on minio, stores the attributes
// of the fileID in mongodb and returns the fileID.

func (store Store) Upload(userID string, data *file.File) error {

	wrapper := func(err error) error { return fmt.Errorf("[store][%s]error while uploading file: %w", userID, err) }

	// Check if quota has enough space, otherwise return error.
	var used float64

	err := store.GetAttribute(userID, quota_used, &used)
	if err != nil {
		return wrapper(err)
	}

	size := sizeMB(data.Attributes.Size)
	if (used + size) > store.QuotaLimitMB {
		return wrapper(ErrQuotaFull)
	}

	err = store.SetAttribute(userID, "quota_used", used+size)
	if err != nil {
		return wrapper(err)
	}

	meta, err := json.Marshal(data.Attributes)
	if err != nil {
		return wrapper(err)
	}

	context, cancel := store.DefaultContext()
	defer cancel()

	var bucket string

	err = store.GetAttribute(userID, "bucket", &bucket)
	if err != nil {
		return wrapper(err)
	}

	exists, err := store.BucketExists(context, bucket)
	if err != nil {
		return wrapper(err)
	}

	if !exists {
		err = store.MakeBucket(context, bucket, minio.MakeBucketOptions{})
		if err != nil {
			return wrapper(err)
		}
	}

	options := minio.PutObjectOptions{}

	// Store metadata separately
	name := fmt.Sprintf("%s.%s", data.Attributes.ID, "meta")

	info, err := store.PutObject(context, bucket, name, bytes.NewBuffer(meta), int64(len(meta)), options)
	if err != nil {
		return wrapper(err)
	}

	logrus.WithField("minio_upload_info", info).Trace()

	name = fmt.Sprintf("%s.%s", data.Attributes.ID, "data")

	options = minio.PutObjectOptions{
		ContentType: data.Mime.String(),
	}

	info, err = store.PutObject(context, bucket, name, bytes.NewBuffer(data.Data), int64(data.Attributes.Size), options)

	logrus.WithField("minio_upload_info", info).Trace()

	return err
}

func (store Store) Download(fromID, userID, fileID string) (*file.File, error) {

	wrapper := func(err error) error { return fmt.Errorf("[store][%s]error while dowloading file: %w", userID, err) }

	context, cancel := store.DefaultContext()
	defer cancel()

	var bucket string

	err := store.GetAttribute(userID, "bucket", &bucket)
	if err != nil {
		return nil, wrapper(err)
	}

	exists, err := store.BucketExists(context, bucket)
	if err != nil {
		return nil, wrapper(err)
	}

	if !exists {
		return nil, wrapper(ErrBucketNotPresent)
	}

	options := minio.GetObjectOptions{}

	download := func(suffix string) ([]byte, error) {
		name := fmt.Sprintf("%s.%s", fileID, suffix)

		object, err := store.GetObject(context, bucket, name, options)
		if err != nil {
			return nil, wrapper(err)
		}

		defer object.Close()

		data, err := io.ReadAll(object)
		if err != nil {
			return nil, wrapper(err)
		}
		return data, nil
	}

	meta, err := download("meta")
	if err != nil {
		return nil, wrapper(err)
	}

	data, err := download("data")
	if err != nil {
		return nil, wrapper(err)
	}

	var attributes file.Attributes

	err = json.Unmarshal(meta, &attributes)
	if err != nil {
		return nil, wrapper(err)
	}

	file := file.New(data, attributes.Name, attributes.Perm)

	file.Attributes.ID = fileID

	if !file.UserPermitted(fromID) && userID != fromID {
		return nil, wrapper(ErrUserNotAllowed)
	}

	return &file, nil
}

func (store Store) DeleteOne(userID, fileID string) error {

	wrapper := func(err error) error { return fmt.Errorf("[store][%s]error while deleting file: %w", userID, err) }

	context, cancel := store.DefaultContext()
	defer cancel()

	var bucket string

	err := store.GetAttribute(userID, "bucket", &bucket)
	if err != nil {
		return wrapper(err)
	}

	var used float64

	err = store.GetAttribute(userID, quota_used, &used)
	if err != nil {
		return wrapper(err)
	}

	file, err := store.Download(userID, userID, fileID)
	if err != nil {
		return wrapper(err)
	}

	fileSize := sizeMB(file.Attributes.Size)
	used -= fileSize

	// Update quota
	err = store.SetAttribute(userID, quota_used, used)
	if err != nil {
		return wrapper(err)
	}

	remove := func(suffix string) error {

		name := fmt.Sprintf("%s.%s", fileID, suffix)

		options := minio.RemoveObjectOptions{}

		err = store.RemoveObject(context, bucket, name, options)
		if err != nil {
			return err
		}
		return nil
	}

	err = remove("meta")
	if err != nil {
		return wrapper(err)
	}
	err = remove("data")
	if err != nil {
		return wrapper(err)
	}

	return nil
}

func (store Store) ListAll(userID string) (interface{}, error) {
	type Details struct {
		Name string `json:"name"`
		Size int    `json:"size"`
	}

	wrapper := func(err error) error { return fmt.Errorf("[store][%s]error while getting file list: %w", userID, err) }

	context, cancel := store.DefaultContext()
	defer cancel()

	var bucket string

	err := store.GetAttribute(userID, "bucket", &bucket)
	if err != nil {
		return nil, wrapper(err)
	}

	objects_list := make(map[string]Details)

	options := minio.ListObjectsOptions{}

	objects := store.ListObjects(context, bucket, options)

	get_suffix := func(name string) string {
		strings := strings.Split(name, ".")
		return strings[len(strings)-1]
	}

	for object := range objects {
		suffix := get_suffix(object.Key)

		if suffix != "meta" {
			continue
		}

		options := minio.GetObjectOptions{}

		metadata, err := store.GetObject(context, bucket, object.Key, options)
		if err != nil {
			return nil, wrapper(err)
		}

		data, err := io.ReadAll(metadata)
		if err != nil {
			return nil, wrapper(err)
		}

		metadata.Close()

		var attributes file.Attributes
		err = json.Unmarshal(data, &attributes)

		if err != nil {
			return nil, wrapper(err)
		}

		objects_list[attributes.ID] = Details{
			Name: attributes.Name,
			Size: attributes.Size,
		}
	}

	return objects_list, nil
}

func (store Store) GetFileAttributes(userID, fileID string) (file.Attributes, error) {
	wrapper := func(err error) error {
		return fmt.Errorf("[store][%s]error while getting file attributes: %w", userID, err)
	}

	context, cancel := store.DefaultContext()
	defer cancel()

	var bucket string

	err := store.GetAttribute(userID, "bucket", &bucket)
	if err != nil {
		return file.Attributes{}, wrapper(err)
	}

	name := fmt.Sprintf("%s.%s", fileID, "meta")

	options := minio.GetObjectOptions{}

	object, err := store.GetObject(context, bucket, name, options)
	if err != nil {
		return file.Attributes{}, wrapper(err)
	}

	defer object.Close()

	data, err := io.ReadAll(object)
	if err != nil {
		return file.Attributes{}, wrapper(err)
	}

	var attributes file.Attributes

	err = json.Unmarshal(data, &attributes)
	if err != nil {
		return file.Attributes{}, wrapper(err)
	}

	return attributes, nil
}
func (store Store) SetFileAttributes(userID, fileID string, attributes file.Attributes) error {
	wrapper := func(err error) error {
		return fmt.Errorf("[store][%s]error while setting file attributes: %w", userID, err)
	}

	context, cancel := store.DefaultContext()
	defer cancel()

	var bucket string

	err := store.GetAttribute(userID, "bucket", &bucket)
	if err != nil {
		return wrapper(err)
	}

	data, err := json.Marshal(attributes)
	if err != nil {
		return wrapper(err)
	}

	name := fmt.Sprintf("%s.%s", fileID, "meta")

	err = store.RemoveObject(context, bucket, name, minio.RemoveObjectOptions{})
	if err != nil {
		return wrapper(err)
	}

	_, err = store.PutObject(context, bucket, name, bytes.NewBuffer(data), int64(len(data)), minio.PutObjectOptions{})
	if err != nil {
		return wrapper(err)
	}

	return err
}

func (store Store) DeleteAll(userID string) error {

	wrapper := func(err error) error { return fmt.Errorf("[store][%s]error while deleting storage: %w", userID, err) }

	context, cancel := store.DefaultContext()
	defer cancel()

	var bucket string

	err := store.GetAttribute(userID, "bucket", &bucket)
	if err != nil {
		return wrapper(err)
	}

	optionsList := minio.ListObjectsOptions{}

	objects := store.ListObjects(context, bucket, optionsList)

	optionsRemove := minio.RemoveObjectsOptions{}

	errors := store.RemoveObjects(context, bucket, objects, optionsRemove)

	for err := range errors {
		if err.Err != nil {
			return wrapper(err.Err)
		}
	}

	err = store.RemoveBucket(context, bucket)
	if err != nil {
		return wrapper(err)
	}

	return nil
}
