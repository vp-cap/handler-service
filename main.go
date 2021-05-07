package main

import (
	"context"
	"io/ioutil"
	"log"
	"os"
	"os/exec"

	database "github.com/vp-cap/data-lib/database"
	storage "github.com/vp-cap/data-lib/storage"
	config "github.com/vp-cap/handler-service/config"
	pbModel "github.com/vp-cap/handler-service/genproto/models"
	pb "github.com/vp-cap/handler-service/genproto/task"

	"google.golang.org/protobuf/proto"
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

func init() {
	var err error
	configs, err = config.GetConfigs()
	if err != nil {
		log.Println("Unable to get config")
	}
}

// call the required binary
func processVideo(ctx context.Context, videoCid string) bool {
	// If its already there, ignore
	_, err := db.GetVideoInference(ctx, videoCid)
	if err == nil {
		log.Printf("Video Inference for %s already exists", videoCid)
		return true
	}

	err = store.GetVideo(ctx, videoCid, VideoLocation)
	if err != nil {
		log.Println(err)
		return false
	}
	log.Println("Fetched video from storage")

	var argsList []string

	log.Println("Processing Video....")
	argsList = append(argsList, "./process_video.py", VideoLocation)
	cmd := exec.Command("python", argsList...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()

	if err != nil {
		log.Println("Unable to run the process:", err)
		return false
	}
	log.Println("Video Inference completed")

	in, _ := ioutil.ReadFile(VideoInferenceLocation)
	videoInference := &pbModel.VideoInference{}
	if err = proto.Unmarshal(in, videoInference); err != nil {
		log.Println("Failed to parse address book:", err)
		return false
	}
	dbVideoInference := database.VideoInference{}
	dbVideoInference.Name = videoCid
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

	if err != nil {
		log.Println("Unable to store video in db:", err)
		return false
	}

	log.Println("Video Inference stored in DB")
	return true
}

func main() {
	// Enable line numbers in logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	ctx := context.Background()
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()

	// DB and store
	// TODO add connection close method in the interface of db and store
	var err error
	db, err = database.GetDatabaseClient(ctx, configs.Database)
	if err != nil {
		log.Fatalln("Unable to connect to DB", err)
	}
	log.Println("Connected to DB")
	store, err = storage.GetStorageClient(configs.Storage)
	if err != nil {
		log.Fatalln("Unable to connect to Storage", err)
	}
	log.Println("Connected to Storage")

	conn, err := getTaskQueueConnection()
	if err != nil {
		log.Fatalln("Unable to connect to Storage", err)
	}
	defer conn.Close()
	msgs, err := getChannelForMessages(conn)
	if (err != nil) {
		log.Fatalln("Failed to connect to task queue", err)
	}
	log.Println("Connected to the task queue")

	forever := make(chan bool)
	// Look out for messages from the task queue.
	go func() {
		for d := range msgs {
			task := &pb.Task{}
			if err := proto.Unmarshal(d.Body, task); err != nil {
				log.Fatalln("Failed to parse:", err)
			}
			log.Println(task)
			log.Println("Task Received: ", task.VideoName)

			if processVideo(ctx, task.VideoCid) {
				log.Printf("Done")
				d.Ack(false)
			} else {
				// requeue
				d.Nack(false, true)
			}
		}
	}()
	<-forever
}
