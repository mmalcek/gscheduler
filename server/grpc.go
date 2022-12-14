package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	pb "github.com/mmalcek/gscheduler/proto/go"
	"google.golang.org/grpc"
	codes "google.golang.org/grpc/codes" // https://grpc.github.io/grpc/core/md_doc_statuscodes.html
	"google.golang.org/grpc/credentials"
	status "google.golang.org/grpc/status"
)

type server struct {
	pb.UnimplementedTaskManagerServer
}

func grpcServer() {
	var s *grpc.Server
	lis, err := net.Listen("tcp", net.JoinHostPort(config.ServerAddress, config.ServerPort))
	if err != nil {
		logger.Errorf("grpcServer-Listen: %v", err)
		os.Exit(1)
	}

	if config.SSL.CRT != "" && config.SSL.KEY != "" {
		fmt.Println("TLS enabled")
		cert, err := tls.LoadX509KeyPair(config.SSL.CRT, config.SSL.KEY)
		if err != nil {
			logger.Errorf("grpcServer-LoadX509KeyPair: ", err.Error())
			os.Exit(1)
		}

		// Choose client authentication method
		clientCert := tls.ClientAuthType(tls.NoClientCert)
		if config.SSL.ClientCert {
			clientCert = tls.ClientAuthType(tls.RequireAndVerifyClientCert)
		}

		// Load CA certificate. If no certificate is provided, server will use OS certStore.
		var certPool *x509.CertPool
		if config.SSL.CA != "" {
			caCert, err := os.ReadFile(config.SSL.CA)
			if err != nil {
				logger.Errorf("grpcServer-readCertFile: ", err.Error())
				os.Exit(1)
			}
			certPool = x509.NewCertPool()
			if ok := certPool.AppendCertsFromPEM(caCert); !ok {
				logger.Errorf("grpcServer-certificateError")
				os.Exit(1)
			}
		}
		// Create new server
		s = grpc.NewServer(
			grpc.MaxSendMsgSize(1024*1024*200), // 200MB max message size (because logFiles can be big)
			grpc.Creds(credentials.NewTLS(&tls.Config{
				Certificates: []tls.Certificate{cert},
				ClientAuth:   clientCert,
				ClientCAs:    certPool,
			})))
	} else {
		fmt.Println("TLS disabled")
		s = grpc.NewServer(grpc.MaxSendMsgSize(1024 * 1024 * 200)) // 200MB max message size (because logFiles can be big)
	}

	pb.RegisterTaskManagerServer(s, &server{})
	logger.Infof("Starting gRPC server: %v", net.JoinHostPort(config.ServerAddress, config.ServerPort))
	if err := s.Serve(lis); err != nil {
		logger.Errorf("grpcServer-failedToStart: %s", err.Error())
		os.Exit(1)
	}
}

// List all apps from config
func (s *server) AppsList(ctx context.Context, in *pb.Empty) (*pb.List, error) {
	apps := &pb.List{}
	for key := range config.Apps {
		apps.Data = append(apps.Data, key)
	}
	return apps, nil
}

// Create new task and return UUID
func (s *server) TaskCreate(ctx context.Context, task *pb.Task) (*pb.Status, error) {
	taskUUID, err := tasks.create(task)
	if err != nil {
		return &pb.Status{Message: "failed", Uuid: taskUUID}, status.Newf(codes.Unknown, err.Error()).Err()
	}
	return &pb.Status{Message: "success", Uuid: taskUUID}, nil
}

// Update task configuration
func (s *server) TaskUpdate(ctx context.Context, task *pb.Task) (*pb.Status, error) {
	if err := tasks.update(task); err != nil {
		return &pb.Status{Message: "failed", Uuid: task.GetUuid()}, status.Newf(codes.Unknown, err.Error()).Err()
	}
	return &pb.Status{Message: "success", Uuid: task.GetUuid()}, nil
}

// Delete task
func (s *server) TaskDelete(ctx context.Context, task *pb.TaskUUID) (*pb.Status, error) {
	if err := tasks.delete(task.GetUuid()); err != nil {
		return &pb.Status{Message: "failed", Uuid: task.GetUuid()}, status.Newf(codes.Unknown, err.Error()).Err()
	}
	return &pb.Status{Message: "success", Uuid: task.GetUuid()}, nil
}

// Stop task (Force = true will kill the task)
func (s *server) TaskStop(ctx context.Context, in *pb.TaskUUID) (*pb.Status, error) {
	if err := tasks.stop(in.GetUuid(), in.GetForce()); err != nil {
		return &pb.Status{Message: "failed", Uuid: in.GetUuid()}, status.Newf(codes.Unknown, err.Error()).Err()
	}
	return &pb.Status{Message: "success", Uuid: in.GetUuid()}, nil
}

// Start task
func (s *server) TaskStart(ctx context.Context, in *pb.TaskUUID) (*pb.Status, error) {
	if err := tasks.start(in.GetUuid()); err != nil {
		return &pb.Status{Message: "failed", Uuid: in.GetUuid()}, status.Newf(codes.Unknown, err.Error()).Err()
	}
	return &pb.Status{Message: "success", Uuid: in.GetUuid()}, nil
}

// List all tasks
func (s *server) TasksList(ctx context.Context, in *pb.Empty) (*pb.Tasks, error) {
	return &pb.Tasks{Tasks: tasks.getAll()}, nil
}

// Run task once/immediately (Runs in go routine - caller can watch status in SchedulerWatch)
func (s *server) TaskRun(ctx context.Context, in *pb.TaskUUID) (*pb.Status, error) {
	task := tasks.get(in.GetUuid())
	if task == nil {
		return &pb.Status{Message: "failed", Uuid: in.GetUuid()}, status.Newf(codes.InvalidArgument, "notFound").Err()
	}
	if tasksCTX.get(in.GetUuid()).ctx != nil {
		return &pb.Status{Message: "failed", Uuid: in.GetUuid()}, status.Newf(codes.FailedPrecondition, "alreadyRunning").Err()
	}
	go func() {
		scheduler.taskJob(task)()
	}()
	return &pb.Status{Message: "success", Uuid: in.GetUuid()}, nil
}

// Stop task scheduler (force = kill all tasks immediately)
func (s *server) SchedulerStop(ctx context.Context, in *pb.Stop) (*pb.Status, error) {
	if err := scheduler.stop(in.GetForce()); err != nil {
		return &pb.Status{Message: "failed"}, status.Newf(codes.Unknown, err.Error()).Err()
	}
	return &pb.Status{Message: "success"}, nil
}

// Start the scheduler
func (s *server) SchedulerStart(ctx context.Context, in *pb.Empty) (*pb.Status, error) {
	if err := scheduler.start(); err != nil {
		return &pb.Status{Message: "failed"}, status.Newf(codes.Unknown, err.Error()).Err()
	}
	return &pb.Status{Message: "success"}, nil
}

// Watch all tasks events as they happen (stream)
func (s *server) SchedulerWatch(in *pb.Empty, stream pb.TaskManager_SchedulerWatchServer) error {
	chanUUID := uuid.New().String()
	logWatchChans.add(chanUUID, 0)
	defer logWatchChans.delete(chanUUID)
	for {
		select {
		case change := <-logWatchChans.get(chanUUID):
			if err := stream.Send(change.(*pb.TaskLog)); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return nil
		}
	}
}

// Return UUIDs of currently running tasks
func (s *server) SchedulerRunningTasks(ctx context.Context, in *pb.Empty) (*pb.List, error) {
	tsk := make([]string, 0)
	for key := range tasksCTX.getAll() { // If channel exists task is running
		tsk = append(tsk, key)
	}
	return &pb.List{Data: tsk}, nil
}

// Exec single command without using scheduler
func (s *server) ExecCmd(ctx context.Context, in *pb.Task) (*pb.ExecStatus, error) {
	return execCommand(in)
}

// List all logFiles (returns datemarks 20060102)
func (s *server) LogList(ctx context.Context, in *pb.Empty) (*pb.List, error) {
	return logListCreate()
}

// Get logFile. Expects datemark 20060102
func (s *server) LogGet(ctx context.Context, in *pb.Request) (*pb.File, error) {
	file, err := os.ReadFile(filepath.Join(config.LogFolder, fmt.Sprintf("log_%s.yaml", in.GetMsg())))
	if err != nil {
		return nil, status.Newf(codes.NotFound, "fileNotFound").Err()
	}
	return &pb.File{Content: file}, nil
}
