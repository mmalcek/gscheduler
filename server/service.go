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
	config        = tConfig{ServerAddress: "127.0.0.1", ServerPort: "50051", LogLimit: -1, Apps: map[string]string{}}
	scheduler     = tCron{cron: cron.New(cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)))}
	tasks         = &tTasks{tasks: make([]*pb.Task, 0)}
	tasksCTX      = tTasksCtxMap{taskCtx: make(map[string]*tTaskState, 0)}
	taskLog       = make(chan *pb.TaskLog, 100)
	logWatchChans = tSyncChanMap{channels: make(map[string]chan interface{})} // Send taskLog to this chan
)

func (p *program) run() {
	if err := config.loadConfig(); err != nil {
		logger.Errorf("configLoadFailed: %s", err.Error())
	}
	config.fixConfigPaths()   // Fix paths in config (relative to absolute)
	go tasksLogWatch(taskLog) // Watch tasks (stdOut,stdErr) channel. Send to logWatchChans and write to fileLog
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
