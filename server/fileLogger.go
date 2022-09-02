package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	pb "github.com/mmalcek/gscheduler/proto/go"
	"gopkg.in/yaml.v3"
)

const TASK_LOG_NAME = "log_20060102.yaml"

var tasksLogFile *os.File

func taskLogToFile(logData *pb.TaskLog) error {
	if config.LogFolder == "" { // If no log folder, do not log to file
		return nil
	}
	if !filepath.IsAbs(config.LogFolder) {
		config.LogFolder = filepath.Join(filepath.Dir(os.Args[0]), config.LogFolder)
	}
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
			if err := os.Mkdir(config.LogFolder, os.ModePerm); err != nil {
				return err
			}
		}
		if tasksLogFile, err = os.OpenFile(filepath.Join(config.LogFolder, fileName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err != nil {
			return err
		}
		go deleteTasksLogFiles()
	}
	return nil
}

// Delete log files older than config.LogLimit when date change or when app strats
func deleteTasksLogFiles() {
	if config.LogLimit < 1 {
		return // No limit
	}
	files, err := os.ReadDir(config.LogFolder)
	if err != nil {
		logger.Error(fmt.Errorf("deleteTasksLogFiles-ReadDir: %s", err.Error()))
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
				logger.Error(fmt.Errorf("deleteTasksLogFiles-Remove: %s", err.Error()))
			}
		}
	}
}

// Create list of log files
func logListCreate() (list *pb.List, err error) {
	if config.LogFolder != "" {
		list = &pb.List{}
		files, err := os.ReadDir(config.LogFolder)
		if err != nil {
			return nil, err
		}
		for i := range files {
			if files[i].IsDir() {
				continue
			}
			if strings.Split(files[i].Name(), "_")[0] == "log" {
				_, err := time.Parse(TASK_LOG_NAME, files[i].Name())
				if err != nil { // Not a date file
					continue
				}
				list.Data = append(
					list.Data,
					strings.Replace(strings.Replace(files[i].Name(), ".yaml", "", 1), "log_", "", 1))
			}
		}
	}
	return list, nil
}
