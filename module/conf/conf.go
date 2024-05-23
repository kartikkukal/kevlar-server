package conf

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"kevlar/module/attr"
	"kevlar/module/auth"
	"kevlar/module/db/minio"
	"kevlar/module/db/mongo"
	"kevlar/module/log"
	"kevlar/module/store"
	"os"

	"github.com/creasty/defaults"
)

func PrettifyJSON(data []byte) []byte {
	var output bytes.Buffer
	err := json.Indent(&output, data, "", "		")
	if err != nil {
		return nil
	}
	return output.Bytes()
}

type Http struct {
	Address    string `default:":8080"`
	DomainName string `default:"example.com"`
}

type RootConfig struct {
	Mongo mongo.Config
	Log   log.Config
	Auth  auth.Config
	Http  Http
	Minio minio.Config
	Attr  attr.Config
	Store store.Config
}

const (
	configDir  = "kevlar/"
	configName = "main.conf"
	configPath = configDir + configName
	configPerm = 0744
)

func Read() (RootConfig, error) {
	config := RootConfig{}
	writeDefault := func() (RootConfig, error) {
		err := defaults.Set(&config)
		if err != nil {
			return RootConfig{}, err
		}
		data, err := json.Marshal(config)
		if err != nil {
			return RootConfig{}, err
		}
		err = os.MkdirAll(configDir, configPerm)
		if err != nil {
			return RootConfig{}, err
		}
		data = PrettifyJSON(data)
		return config, ioutil.WriteFile(configPath, data, configPerm)
	}
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return writeDefault()
	}
	err = json.Unmarshal(data, &config)
	if err != nil {
		return writeDefault()
	}
	return config, err
}
