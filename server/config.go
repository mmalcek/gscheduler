package main

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type (
	tConfig struct {
		ServerAddress string `yaml:"server_address"`
		ServerPort    string `yaml:"server_port"`
		TasksFile     string `yaml:"tasks_file"`
		LogFolder     string `yaml:"log_folder"`
		LogLimit      int    `yaml:"log_limit"`
		SSL           struct {
			CRT        string `yaml:"crt"`
			KEY        string `yaml:"key"`
			CA         string `yaml:"ca"`
			ClientCert bool   `yaml:"client_cert"`
		} `yaml:"ssl"`
		Apps map[string]string `yaml:"apps"`
	}
)

var config tConfig

func configInit() error {
	config.ServerAddress = "127.0.0.1"
	config.ServerPort = "50051"
	config.LogFolder = ""
	config.LogLimit = -1
	config.Apps = make(map[string]string, 0)
	if err := getConfigFile(); err != nil {
		return err
	}
	return nil
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
