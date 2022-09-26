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

func (c *tConfig) loadConfig() error {
	configData, err := os.ReadFile(filepath.Join(filepath.Dir(os.Args[0]), "config.yaml"))
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(configData, &c); err != nil {
		return err
	}
	return nil
}

func (c *tConfig) fixConfigPaths() {
	if config.TasksFile == "" {
		c.TasksFile = filepath.Join(filepath.Dir(os.Args[0]), "tasks.yaml")
	}
	if !filepath.IsAbs(c.TasksFile) {
		c.TasksFile = filepath.Join(filepath.Dir(os.Args[0]), c.TasksFile)
	}
	if !filepath.IsAbs(c.LogFolder) {
		c.LogFolder = filepath.Join(filepath.Dir(os.Args[0]), c.LogFolder)
	}
	for k, v := range config.Apps {
		if !filepath.IsAbs(v) {
			c.Apps[k] = filepath.Join(filepath.Dir(os.Args[0]), v)
		}
	}
}

func (c *tConfig) addApp(name string, app string) error {
	if err := config.loadConfig(); err != nil {
		logger.Errorf("configLoadFailed: %s", err.Error())
	}
	app = filepath.ToSlash(app)
	c.Apps[name] = app
	configData, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(os.Args[0]), "config.yaml"), configData, 0644); err != nil {
		return err
	}
	return nil
}

func (c *tConfig) delApp(name string) error {
	if err := config.loadConfig(); err != nil {
		logger.Errorf("configLoadFailed: %s", err.Error())
	}
	delete(c.Apps, name)
	configData, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(os.Args[0]), "config.yaml"), configData, 0644); err != nil {
		return err
	}
	return nil
}
