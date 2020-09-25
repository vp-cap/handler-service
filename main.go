package main

import (
	"context"
	"log"
	"net"
	"sync"
	"os"
	"os/exec"

	database "cap/data-lib/database"
	storage "cap/data-lib/storage"
	config "cap/handler-service/config"
	pb "cap/handler-service/genproto"

	"google.golang.org/grpc"
	"github.com/golang/protobuf/ptypes/empty"
)

const (
	// VideoProcessingBinaryLocation of the code to run the processing on input video
	VideoProcessingBinaryLocation = "./process_video"
	// VideoLocation stored after pull from storage
	VideoLocation = "./video"
)

var (
	configs config.Configurations
	db      database.Database = nil
	store   storage.Storage   = nil
)

// Handler struct 
type Handler struct {
	mu         sync.Mutex
	addr       string
	taskCid    string
	taskStatus pb.TaskStatus_Status
}

func init() {
	var err error
	configs, err = config.GetConfigs()
	if err != nil {
		log.Println("Unable to get config")
	}
}

// AllocateTask called by task handler to process  
func (h *Handler) AllocateTask(ctx context.Context, task *pb.Task) (*empty.Empty, error) {
	h.taskCid = task.VideoCid
	log.Println("New Task assigned:", h.taskCid)

	err := store.GetVideo(ctx, task.VideoCid, VideoLocation)
	if err != nil {
		h.taskCid = ""
		h.taskStatus = pb.TaskStatus_UNASSIGNED
		log.Println(err)
		return &empty.Empty{}, nil
	}
	log.Println("Video fetched from storage")

	h.taskStatus = pb.TaskStatus_WORKING
	go processVideo();

	return &empty.Empty{}, nil 
}

// GetTaskStatus for the current task
func (h *Handler) GetTaskStatus(ctx context.Context, e *empty.Empty) (*pb.TaskStatus, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return &pb.TaskStatus{Status: h.taskStatus}, nil
}

// call the required binary
func processVideo() {
	// TODO change to binary process for now
	
	var argsList []string

	argsList = append(argsList, "process_video.py", VideoLocation) 
	cmd := exec.Command("python", argsList...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	log.Println("Processing Video....")

	if err != nil {
		log.Println("Unable to run the process")
	}
}

// register this handler on the task-handler service
func (h *Handler) registerOnTaskAllocator() error {
	// connect
	// log.Println(configs.Services)
	conn, err := grpc.Dial(configs.Services.TaskAllocator, grpc.WithInsecure())
	if err != nil {
		log.Println(err)
		return err
	}

	defer conn.Close()

	client := pb.NewRegisterHandlerServiceClient(conn)
	_, err = client.RegisterHandler(context.Background(), &pb.Handler{Addr: h.addr})
	if err != nil {
		log.Println(err)
		return err
	}
	log.Println("Registered on task allocator")
	return nil
}

func main() {
	// Enable line numbers in logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	log.Println(configs.Storage)
	ctx := context.Background()
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()

	// DB and store
	var err error
	db, err = database.GetDatabaseClient(ctx, configs.Database)
	if err != nil {
		log.Fatalln(err)
	}
	store, err = storage.GetStorageClient(configs.Storage)
	if err != nil {
		log.Fatalln(err)
	}

	addr := "localhost:" + configs.Server.Port
	// listen on port 
	lis, err := net.Listen("tcp", ":"+configs.Server.Port)
	if err != nil {
		log.Fatalln(err)
	}

	grpcServer := grpc.NewServer()

	handler := &Handler{
		addr: addr,
		taskCid: "",
		taskStatus: pb.TaskStatus_UNASSIGNED,
	}

	pb.RegisterTaskAllocationServiceServer(grpcServer, handler)

	err = handler.registerOnTaskAllocator()
	if err != nil {
		return
	}
	
	// serve
	log.Println("Serving on", configs.Server.Port)
	grpcServer.Serve(lis)
}