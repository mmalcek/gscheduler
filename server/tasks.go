package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/google/uuid"
	pb "github.com/mmalcek/gscheduler/proto/go"
	"github.com/robfig/cron/v3"
)

type tTasks struct {
	mutex sync.RWMutex
	tasks []*pb.Task
}

// Load tasks from file
func (tsk *tTasks) load() error {
	tsk.mutex.Lock()
	defer tsk.mutex.Unlock()
	// Create file if not exists
	if _, err := os.Stat(config.TasksFile); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(config.TasksFile), 0660); err != nil {
			return fmt.Errorf("createDir: %v", err.Error())
		}
		if _, err := os.Create(config.TasksFile); err != nil {
			return fmt.Errorf("createFile: %v", err.Error())
		}
	}
	// Load tasks from file
	configData, err := os.ReadFile(config.TasksFile)
	if err != nil {
		return fmt.Errorf("openFile: %v", err.Error())
	}
	if err := yaml.Unmarshal(configData, &tsk.tasks); err != nil {
		return fmt.Errorf("unmarshal: %v", err.Error())
	}
	// Validate data in tasks
	for i := range tsk.tasks {
		if err := tsk.validateInput(tsk.tasks[i]); err != nil {
			return fmt.Errorf("validateInput: %s, err: %s", tsk.tasks[i].GetName(), err.Error())
		}
		if err := tasks.validateUUID(tsk.tasks[i].Uuid); err != nil {
			return fmt.Errorf("taskUUID: %s, err: %s", tsk.tasks[i].GetName(), err.Error())
		}
	}
	return nil
}

// Get single task by UUID
func (tsk *tTasks) get(uuid string) *pb.Task {
	tsk.mutex.RLock()
	defer tsk.mutex.RUnlock()
	for i := range tsk.tasks {
		if tsk.tasks[i].Uuid == uuid {
			return tsk.tasks[i]
		}
	}
	return nil
}

// Get all tasks
func (tsk *tTasks) getAll() []*pb.Task {
	tsk.mutex.RLock()
	defer tsk.mutex.RUnlock()
	return tsk.tasks
}

// Create new task
func (tsk *tTasks) create(task *pb.Task) (taskUUID string, err error) {
	tsk.mutex.Lock()
	defer tsk.mutex.Unlock()
	if err := tsk.validateInput(task); err != nil {
		return "", fmt.Errorf("validateInput: %s", err.Error())
	}
	/*
		for i := range tsk.tasks {
			if tsk.tasks[i].GetName() == task.GetName() {
				return "", fmt.Errorf("taskNameExists")
			}
		}
	*/
	task.Uuid = uuid.New().String()
	task.CronId = 0
	task.Enabled = false
	tsk.tasks = append(tsk.tasks, task)
	tsk.saveTasks()
	return task.Uuid, nil
}

func (tsk *tTasks) start(uuid string) error {
	tsk.mutex.Lock()
	defer tsk.mutex.Unlock()
	for i := range tsk.tasks {
		if tsk.tasks[i].GetUuid() == uuid {
			tsk.tasks[i].Enabled = true
			if tsk.tasks[i].CronId != 0 {
				return fmt.Errorf("taskAlreadyStarted")
			}
			var err error
			if tsk.tasks[i].CronId, err = scheduler.addTask(tsk.tasks[i]); err != nil {
				return fmt.Errorf("addTask: %s, err: %s", tsk.tasks[i].Uuid, err.Error())
			}
			tsk.saveTasks()
			taskLog <- &pb.TaskLog{Name: tsk.tasks[i].Name, Tags: tsk.tasks[i].Tags, Uuid: tsk.tasks[i].Uuid, Message: "taskStart", Type: "sys", Timestamp: time.Now().UnixMicro()}
			return nil
		}
	}
	return fmt.Errorf("taskNotFound")
}

func (tsk *tTasks) stop(tuuid string, force bool) error {
	// if task running (has context) stop first. force=kill immediately
	if tasksCTX.get(tuuid) != nil {
		if force { // Find the task in chain that is currently running and kill it
			lastTask := tsk.get(tuuid)
			for lastTask.NextTask != "" && tasksCTX.get(lastTask.NextTask) != nil {
				lastTask = tsk.get(lastTask.NextTask)
			}
			tasksCTX.get(lastTask.Uuid).cancel()
		} else { // wait in loop until task is finished
			taskRunning := true
			for taskRunning {
				if tasksCTX.get(tuuid) == nil {
					taskRunning = false
				}
				time.Sleep(200 * time.Millisecond)
			}
		}
	}
	// remove task from scheduler, set ID to 0 and enabled to false
	tsk.mutex.Lock()
	defer tsk.mutex.Unlock()
	for i := range tsk.tasks {
		if tsk.tasks[i].GetUuid() == tuuid {
			if tsk.tasks[i].CronId == 0 {
				return fmt.Errorf("taskNotRunning")
			}
			scheduler.remove(tsk.tasks[i].CronId)
			tsk.tasks[i].CronId = 0
			tsk.tasks[i].Enabled = false
			tsk.saveTasks()
			taskLog <- &pb.TaskLog{Name: tsk.tasks[i].Name, Tags: tsk.tasks[i].Tags, Uuid: tsk.tasks[i].Uuid, Message: "taskStop", Type: "sys", Timestamp: time.Now().UnixMicro()}
			return nil
		}
	}
	return fmt.Errorf("taskNotFound")
}

// Edit existing task
func (tsk *tTasks) update(task *pb.Task) error {
	tsk.mutex.Lock()
	defer tsk.mutex.Unlock()
	if err := tsk.validateInput(task); err != nil {
		return fmt.Errorf("validateInput: %s, err: %s", task.GetName(), err.Error())
	}
	/*
		for i := range tsk.tasks {
			if tsk.tasks[i].GetName() == task.GetName() && tsk.tasks[i].GetUuid() != task.GetUuid() {
				return fmt.Errorf("taskNameExists")
			}
		}
	*/
	for i := range tsk.tasks {
		if tsk.tasks[i].GetUuid() == task.GetUuid() {
			// Caller should stop task first and decide to let task finish or force stop
			if tsk.tasks[i].CronId != 0 || tsk.tasks[i].Enabled {
				return fmt.Errorf("taskMustBeStopped")
			}
			tsk.tasks[i] = task // Replace task
			tsk.tasks[i].CronId = 0
			tsk.tasks[i].Enabled = false
			tsk.saveTasks()
			return nil
		}
	}
	return fmt.Errorf("taskNotFound")
}

// Delete task
func (tsk *tTasks) delete(uuid string) error {
	tsk.mutex.Lock()
	defer tsk.mutex.Unlock()
	if err := tsk.validateUUID(uuid); err != nil {
		return fmt.Errorf("validateUUID: %v", err.Error())
	}
	for i := range tsk.tasks {
		if tsk.tasks[i].GetUuid() == uuid {
			// Caller should stop task first and decide to let task finish or force stop
			if tsk.tasks[i].CronId != 0 || tsk.tasks[i].Enabled {
				return fmt.Errorf("taskMustBeStopped")
			}
			scheduler.remove(tsk.tasks[i].CronId)
			tsk.tasks = append(tsk.tasks[:i], tsk.tasks[i+1:]...)
			tsk.saveTasks()
			return nil
		}
	}
	return fmt.Errorf("taskNotFound")
}

// Save tasks to file (If save tasks fails it's always Fatal to avoid data inconsistency)
func (tsk *tTasks) saveTasks() {
	tasksData, err := yaml.Marshal(tsk.tasks)
	if err != nil {
		log.Fatalf("taskFileMarshal: %s", err.Error())
	}
	err = os.WriteFile(config.TasksFile, tasksData, 0644)
	if err != nil {
		log.Fatalf("taskFileWrite: %s", err.Error())
	}
}

// Save tasks to file with mutex (If save tasks fails it's always Fatal to avoid data inconsistency)
func (tsk *tTasks) saveTasksMutex() {
	tsk.mutex.Lock()
	defer tsk.mutex.Unlock()
	tasksData, err := yaml.Marshal(tsk.tasks)
	if err != nil {
		log.Fatalf("taskFileMarshal: %s", err.Error())
	}
	err = os.WriteFile(config.TasksFile, tasksData, 0644)
	if err != nil {
		log.Fatalf("taskFileWrite: %s", err.Error())
	}
}

// Reset cronID in tasks struct to 0 - means task is not running
func (tsk *tTasks) resetCronID() {
	tsk.mutex.Lock()
	defer tsk.mutex.Unlock()
	for i := range tsk.tasks {
		tsk.tasks[i].CronId = 0
	}
	tsk.saveTasks()
}

func (tsk *tTasks) validateInput(task *pb.Task) error {
	// Validate task name
	if task.GetName() == "" {
		return fmt.Errorf("errName-empty")
	}
	matchName, err := regexp.MatchString(`^[A-Za-z0-9_ ]+$`, task.GetName())
	if err != nil {
		return fmt.Errorf("errName-%s", err.Error())
	}
	if !matchName {
		return fmt.Errorf("errName-only[A-Za-z0-9]_spaceAllowed")
	}
	if len(task.GetName()) > 128 {
		return fmt.Errorf("errName-max128chars")
	}
	// Validate Next task name
	if task.GetNextTask() != "" {
		err := tsk.validateUUID(task.GetNextTask())
		if err != nil {
			return fmt.Errorf("errNextTask-%s", err.Error())
		}
	}
	// Validate description
	matchDesc, err := regexp.MatchString(`^[A-Za-z0-9_ ]+$|^$`, task.GetDescription())
	if err != nil {
		return fmt.Errorf("errDescription-%s", err.Error())
	}
	if !matchDesc {
		return fmt.Errorf("errDescription-only[A-Za-z0-9]_spaceAllowed")
	}
	if len(task.GetDescription()) > 256 {
		return fmt.Errorf("errDescription-max256chars")
	}
	// Validate schedule
	_, err = cron.ParseStandard(task.GetSchedule())
	if err != nil {
		return fmt.Errorf("errSchedule-%s", err.Error())

	}
	// Validate timeout
	if task.GetTimeout() < 1 {
		return fmt.Errorf("errTimeout-min1sec")
	}
	// Validate app
	if task.GetApp() == "" {
		return fmt.Errorf("errApp-empty")
	}
	knownApp := false
	for key := range config.Apps {
		if key == task.GetApp() {
			knownApp = true
			break
		}
	}
	if !knownApp {
		return fmt.Errorf("errApp-missingInConfig")
	}

	// Validate UUID
	if task.GetUuid() != "" {
		if err := tsk.validateUUID(task.GetUuid()); err != nil {
			return fmt.Errorf("errUUID-%s", err.Error())
		}
	}
	return nil
}

func (tsk *tTasks) validateUUID(uuid string) error {
	matchUUID, err := regexp.MatchString(
		`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`,
		uuid)
	if err != nil {
		return err
	}
	if !matchUUID {
		return fmt.Errorf("errInvalidUUIDformat")
	}
	return nil
}
