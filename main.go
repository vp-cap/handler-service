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
	VideoProcessingBinaryLocation = "./process_video.py"
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

// TODO call binary rather than python?
// TODO when updating DB that processing has failed, have another service do clean up, as the it will remain in PROCESSING state forever?
func processVideo(ctx context.Context, videoCid string) bool {
	// If its already there, ignore
	toProcess, err := db.InitializeVideoInference(ctx, videoCid)
	if err != nil {
		log.Println("Error initializing video inference")
		return false
	}
	// Already in processing or complete
	if !toProcess {
		return true;
	}
	
	err = store.GetVideo(ctx, videoCid, VideoLocation)
	if err != nil {
		log.Println("Unable to get Video from storage")
		db.UpdateVideoInference(ctx, database.VideoInference{Id: videoCid, Status: database.STATUS_FAILED})
		return false
	}
	log.Println("Fetched video from storage")

	var argsList []string

	log.Println("Processing Video....")
	argsList = append(argsList, VideoProcessingBinaryLocation, VideoLocation)
	cmd := exec.Command("python", argsList...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()

	if err != nil {
		log.Println("Unable to Process the video")
		db.UpdateVideoInference(ctx, database.VideoInference{Id: videoCid, Status: database.STATUS_FAILED})
		return false
	}
	log.Println("Video Inference obtained")

	in, _ := ioutil.ReadFile(VideoInferenceLocation)
	videoInference := &pbModel.VideoInference{}
	if err = proto.Unmarshal(in, videoInference); err != nil {
		log.Println("Failed to parse video inference")
		db.UpdateVideoInference(ctx, database.VideoInference{Id: videoCid, Status: database.STATUS_FAILED})
		return false
	}
	dbVideoInference := database.VideoInference{}
	dbVideoInference.Id = videoCid
	dbVideoInference.Status = database.STATUS_COMPLETE
	dbVideoInference.ObjectCountsEachSecond = videoInference.ObjectCountsEachSecond
	dbVideoInference.ObjectsToAvgFrequency = videoInference.ObjectsToAvgFrequency
	dbVideoInference.TopFiveObjectsToAvgFrequency = videoInference.TopFiveObjectsToAvgFrequency
	dbVideoInference.TopFiveObjectsToInterval = make(map[string]database.Interval)

	for k, v := range videoInference.TopFiveObjectsToInterval {
		dbVideoInference.TopFiveObjectsToInterval[k] = database.Interval{Start: v.Start, End: v.End}
	}
	log.Println(dbVideoInference)

	// store in the db
	if err = db.UpdateVideoInference(ctx, dbVideoInference); err != nil {
		log.Println("Unable to store video inference in db")
		db.UpdateVideoInference(ctx, database.VideoInference{Id: videoCid, Status: database.STATUS_FAILED})
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
				log.Printf("Done storing video inference")
				d.Ack(false)
			} else {
				log.Printf("Unable to process video")
				// requeue
				d.Nack(false, true)
			}
		}
	}()
	<-forever
}
