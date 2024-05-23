package file

import (
	"io/ioutil"

	"github.com/gabriel-vasile/mimetype"
	"github.com/google/uuid"
)

type Attributes struct {
	Perm []string // holds the list of userIDs that are allowed to use the file
	Size int
	Name string
	ID   string
}

// Basic file with userID permissions
type File struct {
	Data       []byte
	Mime       *mimetype.MIME
	Attributes Attributes
}

func New(data []byte, name string, perm []string) File {
	return File{
		Data: data,
		Mime: mimetype.Detect(data),
		Attributes: Attributes{
			Perm: perm,
			Size: len(data),
			Name: name,
			ID:   uuid.New().String(),
		},
	}
}

func (file *File) Add(userID string) {
	file.Attributes.Perm = append(file.Attributes.Perm, userID)
}

func (file *File) Remove(userID string) {
	for index, value := range file.Attributes.Perm {
		if value == userID {
			file.Attributes.Perm[index] = file.Attributes.Perm[len(file.Attributes.Perm)-1]
			file.Attributes.Perm = file.Attributes.Perm[:len(file.Attributes.Perm)-1]
			break
		}
	}
}

func (file File) UserPermitted(userID string) bool {
	if len(file.Attributes.Perm) == 0 {
		return true
	}
	for _, value := range file.Attributes.Perm {
		if userID == value {
			return true
		}
	}
	return false
}

func WriteFile(file File) error {
	err := ioutil.WriteFile(file.Attributes.Name, file.Data, 0755)
	if err != nil {
		return err
	}
	return nil
}
