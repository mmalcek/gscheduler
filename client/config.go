package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type (
	tConfig struct {
		Server    string `yaml:"server"`
		Port      string `yaml:"port"`
		TLS       bool   `yaml:"tls"`
		CA        string `yaml:"ca"`
		ClientCrt string `yaml:"client_crt"`
		ClientKey string `yaml:"client_key"`
	}
)

var config tConfig

func configInit() {
	config.Server = "127.0.0.1"
	config.Port = "50051"
	config.TLS = false
	config.CA = ""
	if err := getConfigFile(); err != nil {
		fmt.Println("configErr: ", err.Error())
	}
}

func getConfigFile() error {
	configData, err := os.ReadFile(filepath.Join(filepath.Dir(os.Args[0]), "config.yaml"))
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(configData, &config); err != nil {
		return err
	}
	return nil
}
