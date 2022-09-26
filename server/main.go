package main

import (
	"flag"
	"log"
	"os"

	"github.com/kardianos/service"
)

type (
	program struct {
		exit chan struct{}
	}
	tFlags struct {
		svc        *string
		genCrt     *string
		serverName *string
		app        *string
		path       *string
	}
)

var logger service.Logger

func main() {
	flags := tFlags{}
	flags.svc = flag.String("service", "", "Control the system service (start, stop, install, uninstall)")
	flags.genCrt = flag.String("gencrt", "", "Generate SSL certificates")
	flags.serverName = flag.String("server", "", "Server name")
	flags.app = flag.String("app", "", "Application name that should be added to config")
	flags.path = flag.String("path", "", "Application path that should be added to config. If empty -app App will be delted from config")
	flag.Parse()

	if flagProcessed, err := processFlags(flags); err != nil {
		log.Fatal(err)
	} else if flagProcessed {
		os.Exit(0)
	}

	options := make(service.KeyValue)
	options["Restart"] = "on-success"
	options["SuccessExitStatus"] = "1 2 8 SIGKILL"

	svcConfig := &service.Config{
		Name:        "gscheduler",
		DisplayName: "gscheduler",
		Description: "gRPC Task Scheduler",
		Option:      options,
	}
	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}
	logger, err = s.Logger(nil)
	if err != nil {
		log.Fatal(err)
	}

	if len(*flags.svc) != 0 {
		err := service.Control(s, *flags.svc)
		if err != nil {
			logger.Errorf("Valid actions: %q\n", service.ControlAction)
			logger.Errorf(err.Error())
			os.Exit(1)
		}
		return
	}
	err = s.Run()
	if err != nil {
		logger.Error(err)
		os.Exit(1)
	}

}

func processFlags(flags tFlags) (flagProcessed bool, err error) {
	if *flags.genCrt != "" { // Generate SSL certificates
		crtCreate(*flags.genCrt, *flags.serverName)
		return true, nil
	} else if *flags.app != "" {
		if *flags.path != "" { // Add application to config file
			if err := config.addApp(*flags.app, *flags.path); err != nil {
				return false, err
			}
		} else { // Delete application from config file
			if err := config.delApp(*flags.app); err != nil {
				return false, err
			}
		}
		return true, nil
	}
	return false, nil
}
