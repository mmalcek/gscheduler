package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kardianos/service"
	pb "github.com/mmalcek/gscheduler/proto/go"
	"github.com/robfig/cron/v3"
)

var (
	scheduler     = tCron{cron: cron.New(cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)))}
	tasks         = &tTasks{tasks: make([]*pb.Task, 0)}
	tasksCTX      = tTasksCtxMap{taskCtx: make(map[string]*tTaskState, 0)}
	logWatchChans = tSyncChanMap{channels: make(map[string]chan interface{})} // Send taskLog to this chan
	taskLog       = make(chan *pb.TaskLog, 100)
	tasksLogFile  *os.File
)

func (p *program) run() {
	if err := configInit(); err != nil {
		logger.Errorf("loadConfig: %v", err)
		p.Stop(nil)
		os.Exit(1)
	}
	go tasksLogWatch(taskLog) // Watch taskLog (stdOut,stdErr) channel and send taskLog to logWatchChans
	if err := tasks.load(); err != nil {
		logger.Errorf("loadTasks: %v", err.Error())
		p.Stop(nil)
		os.Exit(1)
	}
	if err := scheduler.start(); err != nil {
		logger.Errorf("cronStart: %v", err.Error())
		p.Stop(nil)
		os.Exit(1)
	}
	go grpcServer()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
	<-p.exit
}

func (p *program) Start(s service.Service) error {
	if service.Interactive() {
		logger.Info("Running in terminal.")
	} else {
		logger.Info("Running under service manager.")
	}
	p.exit = make(chan struct{})
	go p.run()
	return nil
}

func (p *program) Stop(s service.Service) error {
	time.Sleep(1 * time.Second)
	func() {
		scheduler.stop(true)
		if tasksLogFile != nil {
			tasksLogFile.Close()
		}
		logger.Info("Stopped")
	}()
	close(p.exit)
	return nil
}
