package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"time"

	pb "github.com/mmalcek/gscheduler/proto/go"
	"github.com/robfig/cron/v3"
)

type tCron struct {
	cron    *cron.Cron
	running bool
}

// Rebuild all tasks and start scheduler
func (cr *tCron) start() error {
	if cr.running {
		return fmt.Errorf("schedulerAlreadyStarted")
	}
	cr.removeAll() // Remove all scheduled tasks (if any)
	var err error
	for _, task := range tasks.getAll() {
		if task.CronId, err = cr.addTask(task); err != nil {
			cr.stop(true) // Forcibly stop all tasks
			return fmt.Errorf("task: %s, err: %s", task.Uuid, err.Error())
		}
	}
	tasks.saveTasksMutex() // Save tasks to file to update cronID
	cr.cron.Start()
	cr.running = true
	return nil
}

// Remove all tasks and stop task scheduler (force=cancell all contexts immediately)
func (cr *tCron) stop(force bool) error {
	if !cr.running {
		return fmt.Errorf("schedulerAlreadyStopped")
	}
	if force {
		for key := range tasksCTX.getAll() { // Force kill all tasks
			tasksCTX.get(key).cancel()
		}
	}
	cr.removeAll()
	<-cr.cron.Stop().Done()
	cr.running = false
	tasks.resetCronID()
	return nil
}

// Remove task from next schedule (not stop currently running task)
func (cr *tCron) remove(entryID int64) {
	cr.cron.Remove(cron.EntryID(entryID))
}

// Remove all scheduled tasks (not stop currently running task)
func (cr *tCron) removeAll() {
	for _, entry := range cr.cron.Entries() {
		cr.cron.Remove(entry.ID)
	}
}

// Add new task to schedule
func (cr *tCron) addTask(task *pb.Task) (cronid int64, err error) {
	if !task.GetEnabled() {
		return 0, nil // Task is disabled
	}
	fmt.Println("Adding task:", task.GetName()) // TODO - remove
	id, err := cr.cron.AddFunc(task.GetSchedule(), cr.taskJob(task))
	if err != nil {
		return 0, err
	}
	return int64(id), nil
}

func (cr *tCron) taskJob(task *pb.Task) func() {
	return func() {
		// If context exists - task is already running
		if tasksCTX.get(task.GetUuid()) != nil {
			taskLog <- genMsg(task, "alreadyRunning", "error")
			return
		}
		// Create context for task - this allows call cancel context and also detect if task is currently running
		tasksCTX.add(task.GetUuid(), task.GetTimeout())
		defer tasksCTX.cancel(task.GetUuid())

		// Run task with context
		cmd := exec.CommandContext(tasksCTX.get(task.GetUuid()).ctx, config.Apps[task.GetApp()], task.GetArgs()...)
		stdoutIn, err := cmd.StdoutPipe()
		if err != nil {
			taskLog <- genMsg(task, fmt.Sprintf("stdoutPipe: %v", err.Error()), "error")
			return
		}
		stderrIn, err := cmd.StderrPipe()
		if err != nil {
			taskLog <- genMsg(task, fmt.Sprintf("stderrPipe: %v", err.Error()), "error")
			return
		}
		if err := cmd.Start(); err != nil {
			taskLog <- genMsg(task, fmt.Sprintf("cmdStart: %v", err.Error()), "error")
			return
		}
		taskLog <- genMsg(task, "started", "info")
		log.Println("Running task:", task.GetName()) // TODO - remove

		// Read stdout and stderr - and wait for task to finish
		var errStdout, errStderr error
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			errStdout = parseStdErrOut(stdoutIn, task, "stdout")
			wg.Done()
		}()
		errStderr = parseStdErrOut(stderrIn, task, "stderr")
		wg.Wait()
		if err = cmd.Wait(); err != nil {
			taskLog <- genMsg(task, err.Error(), "exitStatus") // Maybe replace err.(*exec.ExitError).ExitCode()
		} else {
			taskLog <- genMsg(task, "exit status 0", "exitStatus")
		}
		if errStdout != nil {
			taskLog <- genMsg(task, fmt.Sprintf("stdOutParse: %v", errStdout.Error()), "error")
			return
		}
		if errStderr != nil {
			taskLog <- genMsg(task, fmt.Sprintf("stdErrParse: %v", errStderr.Error()), "error")
			return
		}
		if tasksCTX.get(task.GetUuid()).ctx.Err() != nil {
			taskLog <- genMsg(task, fmt.Sprintf("taskContext: %s", tasksCTX.get(task.GetUuid()).ctx.Err().Error()), "error")
			return
		}

		// If next task is set validate and run it
		if task.GetNextTask() != "" {
			nextTask := tasks.get(task.GetNextTask())
			if nextTask == nil {
				taskLog <- genMsg(task, "nextTaskNotFound", "error")
				return
			}
			if nextTask.GetEnabled() {
				taskLog <- genMsg(task, "nextTaskDisabled", "error") // nextTask must be disabled from schedule
				return
			}
			taskLog <- genMsg(task, "done", "info")
			cr.taskJob(nextTask)() // Main task will finish once "nextTask" is done
			return
		}
		taskLog <- genMsg(task, "done", "info")
	}
}

func parseStdErrOut(r io.Reader, task *pb.Task, msgType string) error {
	buf := make([]byte, 2048)
	for {
		n, err := r.Read(buf[:])
		if n > 0 {
			taskLog <- genMsg(task, string(buf[:n]), msgType)
		}
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return err
		}
	}
}

// Send all task events to all active listeners
// TODO - add logging to file
func tasksLogWatch(event chan *pb.TaskLog) {
	for {
		data := <-event
		if err := taskLogToFile(data); err != nil {
			logger.Errorf("Error writing to LOG: %v", err.Error())
		}
		activeChans := logWatchChans.getAll()
		for i := range activeChans {
			activeChans[i] <- data
		}
	}
}

func genMsg(task *pb.Task, msg string, msgType string) *pb.TaskLog {
	return &pb.TaskLog{
		Name:           task.GetName(),
		SysDescription: task.GetSysDescription(),
		Uuid:           task.GetUuid(),
		Message:        msg,
		Type:           msgType,
		Timestamp:      time.Now().UnixMicro(),
	}
}

// Single command execution without schedule
func execCommand(request *pb.Task) (*pb.ExecStatus, error) {
	taskLog <- &pb.TaskLog{Name: "execCmd", SysDescription: request.GetSysDescription(), Message: "started", Type: "info", Timestamp: time.Now().UnixMicro()}
	var outb, errb bytes.Buffer
	ctx, can := context.WithTimeout(context.Background(), time.Duration(request.GetTimeout())*time.Second)
	defer can()
	cmd := exec.CommandContext(ctx, config.Apps[request.GetApp()], request.GetArgs()...)
	cmd.Stdout = &outb
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		taskLog <- &pb.TaskLog{Name: "execCmd", SysDescription: request.GetSysDescription(), Message: err.Error(), Type: "error", Timestamp: time.Now().UnixMicro()}
		return &pb.ExecStatus{Stdout: "", Stderr: err.Error(), ExitCode: -1}, err
	}
	exitCode := 0
	if err := cmd.Wait(); err != nil {
		exitCode = err.(*exec.ExitError).ExitCode()
	}
	taskLog <- &pb.TaskLog{Name: "execCmd", SysDescription: request.GetSysDescription(), Message: "done", Type: "info", Timestamp: time.Now().UnixMicro()}
	return &pb.ExecStatus{Stdout: outb.String(), Stderr: errb.String(), ExitCode: int64(exitCode)}, nil
}
