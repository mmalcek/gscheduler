package main

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	pb "github.com/mmalcek/gscheduler/proto/go"
	"gopkg.in/yaml.v3"
)

const TASK_LOG_NAME = "log_20060102.yaml"

func taskLogToFile(logData *pb.TaskLog) error {
	if config.LogFolder != "" {
		var err error
		fileName := time.Now().Format(TASK_LOG_NAME)
		if tasksLogFile != nil {
			if filepath.Join(config.LogFolder, fileName) == tasksLogFile.Name() {
				arrLog := []*pb.TaskLog{logData}
				yamlData, err := yaml.Marshal(arrLog)
				if err != nil {
					return err
				}
				tasksLogFile.Write(yamlData)
			} else {
				tasksLogFile.Close()
				if tasksLogFile, err = os.OpenFile(filepath.Join(config.LogFolder, fileName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err != nil {
					return err
				}
				go deleteTasksLogFiles()
			}
		} else {
			if _, err := os.Stat(config.LogFolder); errors.Is(err, os.ErrNotExist) {
				err := os.Mkdir(config.LogFolder, os.ModePerm)
				if err != nil {
					logger.Error(err)
				}
			}
			if tasksLogFile, err = os.OpenFile(filepath.Join(config.LogFolder, fileName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err != nil {
				return err
			}
			go deleteTasksLogFiles()
		}
	}
	return nil
}

func deleteTasksLogFiles() {
	if config.LogLimit < 1 {
		return // No limit
	}
	files, err := os.ReadDir(config.LogFolder)
	if err != nil {
		logger.Error(err)
	}
	allFiles := make([]string, 0)
	for i := range files {
		if files[i].IsDir() {
			continue
		}
		if strings.Split(files[i].Name(), "_")[0] == "log" {
			if _, err := time.Parse(TASK_LOG_NAME, files[i].Name()); err != nil {
				continue // Not a log file
			}
			allFiles = append(allFiles, files[i].Name())
		}
	}
	sort.Strings(allFiles)
	if len(allFiles) > config.LogLimit {
		for _, file := range allFiles[:len(allFiles)-config.LogLimit] {
			err := os.Remove(filepath.Join(config.LogFolder, file))
			if err != nil {
				logger.Error(err)
			}
		}
	}
}
