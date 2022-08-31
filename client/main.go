package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	pb "github.com/mmalcek/gscheduler/proto/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	status "google.golang.org/grpc/status"
)

var (
	act  = flag.String("act", "", "Action to perform")
	file = flag.String("file", "", "File to load")
	task = flag.String("task", "", "Task name")
)

func main() {
	configInit()
	flag.Parse()
	addr := net.JoinHostPort(config.Server, config.Port)
	var conn *grpc.ClientConn
	var err error
	if config.TLS {
		fmt.Println("TLS enabled")
		var certPool *x509.CertPool
		// Load the client certificates from disk. If empty then use the system certificates.
		if config.CA != "" {
			caCert, err := os.ReadFile(config.CA)
			if err != nil {
				log.Fatal("CA-readFile: ", err.Error())
			}
			certPool = x509.NewCertPool()
			if ok := certPool.AppendCertsFromPEM(caCert); !ok {
				log.Fatal("CA-appendCertsFromPEM: ")
			}
		}
		// Load client certificate
		var clientCert tls.Certificate
		if config.ClientCrt != "" && config.ClientKey != "" {
			clientCert, err = tls.LoadX509KeyPair(config.ClientCrt, config.ClientKey)
			if err != nil {
				log.Fatal("clientCert-loadX509KeyPair: ", err.Error())
			}
		}

		if conn, err = grpc.Dial(addr, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			RootCAs:      certPool,
			Certificates: []tls.Certificate{clientCert},
		}))); err != nil {
			log.Fatal("clientCert-dial: ", err.Error())
		}
	} else {
		fmt.Println("TLS disabled")
		conn, err = grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("did not connect: %v", err)
		}
	}
	defer conn.Close()

	c := pb.NewTaskManagerClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*100) // wait for cron to finish
	defer cancel()
	switch *act {
	case "apps":
		r, err := c.AppsList(ctx, &pb.Empty{})
		if err != nil {
			log.Fatalf("AppsList error: %v", err)
		}
		for _, app := range r.GetData() {
			fmt.Printf("%s\n", app)
		}
	case "create":
		tasks, err := loadTasksFromFile(*file)
		if err != nil {
			log.Fatalf("could not load tasks: %v", err)
		}
		for _, task := range tasks {
			r, err := c.TaskCreate(ctx, task)
			if err != nil {
				log.Fatal(parseError(err))
			}
			log.Printf("Task created: %v", r.GetUuid())
		}
	case "update":
		tasks, err := loadTasksFromFile(*file)
		if err != nil {
			log.Fatalf("could not load tasks: %v", err)
		}
		for _, task := range tasks {
			r, err := c.TaskUpdate(ctx, task)
			if err != nil {
				log.Fatal(parseError(err))
			}
			log.Printf("Task updated: %v", r.Message)
		}
	case "delete":
		r, err := c.TaskDelete(ctx, &pb.TaskUUID{Uuid: *task})
		if err != nil {
			log.Fatalf("could not delete task: %v", err.Error())
		}
		log.Printf("Task deleted: %v", r.Message)
	case "stop":
		r, err := c.TaskStop(ctx, &pb.TaskUUID{Uuid: *task, Force: false})
		if err != nil {
			log.Fatalf("could not delete task: %v", err.Error())
		}
		log.Printf("Task deleted: %v", r.Message)
	case "stopForce":
		r, err := c.TaskStop(ctx, &pb.TaskUUID{Uuid: *task, Force: true})
		if err != nil {
			log.Fatalf("could not delete task: %v", err.Error())
		}
		log.Printf("Task deleted: %v", r.Message)
	case "start":
		r, err := c.TaskStart(ctx, &pb.TaskUUID{Uuid: *task})
		if err != nil {
			log.Fatalf("could not start task: %v", err)
		}
		log.Printf("Task started: %v", r.Message)
	case "run":
		r, err := c.TaskRun(ctx, &pb.TaskUUID{Uuid: *task})
		if err != nil {
			log.Fatal(parseError(err))
		}
		log.Printf("TaskResponse: %v", r.Message)
	case "list":
		r, err := c.TasksList(ctx, &pb.Empty{})
		if err != nil {
			log.Fatalf("could not list tasks: %v", err)
		}
		log.Printf("Tasks: %v", r.Tasks)
	case "stopScheduler":
		r, err := c.SchedulerStop(ctx, &pb.Stop{})
		if err != nil {
			log.Fatalf("could not stop scheduler: %v", err)
		}
		log.Printf("Scheduler stopped: %v", r.Message)
	case "stopSchedulerForce":
		r, err := c.SchedulerStop(ctx, &pb.Stop{Force: true})
		if err != nil {
			log.Fatalf("could not stop scheduler: %v", err)
		}
		log.Printf("Scheduler stopped: %v", r.Message)
	case "startScheduler":
		r, err := c.SchedulerStart(ctx, &pb.Empty{})
		if err != nil {
			log.Fatalf("could not start scheduler: %v", err)
		}
		log.Printf("Scheduler started: %v", r.Message)
	case "watch": // watch for new tasks
		r, err := c.SchedulerWatch(context.Background(), &pb.Empty{})
		if err != nil {
			log.Fatalf("could not watch tasks: %v", err)
		}
		for {
			msg, err := r.Recv()
			if err == context.Canceled {
				break
			}
			if err != nil {
				log.Fatalf("could not watch tasks: %v", err)
			}
			fmt.Printf(
				"tsk: %s, t: %s, Type: %s, Msg: %s\n",
				msg.GetName(),
				time.UnixMicro(msg.GetTimestamp()).Format("15:04:05"),
				msg.GetType(),
				msg.GetMessage())
		}
	case "running":
		r, err := c.SchedulerRunningTasks(ctx, &pb.Empty{})
		if err != nil {
			log.Fatalf("could not get scheduler status: %v", err)
		}
		for _, task := range r.GetData() {
			log.Printf("Task: %v", task)
		}
	default:
		log.Fatalf("unknown action: %v", *act)
	}
}

func parseError(err error) string {
	if err == nil {
		return ""
	}
	st, ok := status.FromError(err)
	if ok {
		return fmt.Sprintf("ERR-code: %s, message: %s", st.Code(), st.Message())
	}
	return err.Error()
}
