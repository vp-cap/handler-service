package main

import (
	"context"
	"io/ioutil"
	"log"
	"os/exec"
	"net"
	"os"
	"sync"
	
	database "cap/data-lib/database"
	storage "cap/data-lib/storage"
	config "cap/handler-service/config"
	pb "cap/handler-service/genproto/task"
	pbModel "cap/handler-service/genproto/models"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
)

const (
	// VideoProcessingBinaryLocation of the code to run the processing on input video
	VideoProcessingBinaryLocation = "./process_video"
	// VideoInferenceLocation of the output of the video analysis
	VideoInferenceLocation = "./video_inf"
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
	go h.processVideo(context.Background(), task.VideoName);

	return &empty.Empty{}, nil 
}

// GetTaskStatus for the current task
func (h *Handler) GetTaskStatus(ctx context.Context, e *empty.Empty) (*pb.TaskStatus, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return &pb.TaskStatus{Status: h.taskStatus}, nil
}

// func ObjectMapper(database.VideoInference *obje)

// call the required binary
func (h *Handler) processVideo(ctx context.Context, videoName string) {
	// TODO change to binary process
	var argsList []string

	argsList = append(argsList, "process_video.py", VideoLocation) 
	cmd := exec.Command("python", argsList...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	log.Println("Processing Video....")

	if err != nil {
		log.Println("Unable to run the process:", err)
		h.mu.Lock()
		defer h.mu.Unlock()
		h.taskStatus = pb.TaskStatus_FAILED
		return
	}
	log.Println("Video Inference completed")

	in, err := ioutil.ReadFile(VideoInferenceLocation)
	videoInference := &pbModel.VideoInference{}
	if err := proto.Unmarshal(in, videoInference); err != nil {
		log.Println("Failed to parse address book:", err)
		h.mu.Lock()
		defer h.mu.Unlock()
		h.taskStatus = pb.TaskStatus_FAILED
		return
	}
	dbVideoInference := database.VideoInference{}
	dbVideoInference.Name = videoName
	dbVideoInference.ObjectCountsEachSecond = videoInference.ObjectCountsEachSecond
	dbVideoInference.ObjectsToAvgFrequency = videoInference.ObjectsToAvgFrequency
	dbVideoInference.TopFiveObjectsToAvgFrequency = videoInference.TopFiveObjectsToAvgFrequency
	dbVideoInference.TopFiveObjectsToInterval = make(map[string]database.Interval)

	for k, v := range videoInference.TopFiveObjectsToInterval {
		dbVideoInference.TopFiveObjectsToInterval[k] = database.Interval{Start: v.Start, End: v.End}
	}
	log.Println(dbVideoInference)

	// store in the db
	err = db.InsertVideoInference(ctx, dbVideoInference)

	if (err != nil) {
		log.Println("Unable to store video in db:", err)
		h.mu.Lock()
		defer h.mu.Unlock()
		h.taskStatus = pb.TaskStatus_FAILED
		return
	}

	log.Println("Video Inference stored in DB")
	h.mu.Lock()
	h.taskStatus = pb.TaskStatus_DONE
	h.mu.Unlock()

	// connect
	conn, err := grpc.Dial(configs.Services.TaskAllocator, grpc.WithInsecure())
	if err != nil {
		log.Println("Could not connect to task allocator", err)
		return
	}
	client := pb.NewRegisterHandlerServiceClient(conn)
	_, err = client.RegisterTaskComplete(ctx, &pb.Handler{Addr: h.addr})
	if err != nil {
		log.Println("Could not register task complete", err)
		return
	}
	log.Println("Registered task complete on task allocator")
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