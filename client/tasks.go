package main

import (
	"os"

	"gopkg.in/yaml.v3"

	pb "github.com/mmalcek/gscheduler/proto/go"
)

func loadTasksFromFile(file string) ([]*pb.Task, error) {
	tasks := make([]*pb.Task, 0)
	configData, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(configData, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}
