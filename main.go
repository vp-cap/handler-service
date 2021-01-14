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
	"github.com/streadway/amqp"
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
func processVideo(ctx context.Context, videoName string) bool {
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
		return false
	}
	log.Println("Video Inference completed")

	in, err := ioutil.ReadFile(VideoInferenceLocation)
	videoInference := &pbModel.VideoInference{}
	if err := proto.Unmarshal(in, videoInference); err != nil {
		log.Println("Failed to parse address book:", err)
		return false
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
		return false
	}

	log.Println("Video Inference stored in DB")
	return true;
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

	log.Println(configs.Services.RabbitMq)
	conn, err := amqp.Dial(configs.Services.RabbitMq)
	if err != nil {
		log.Println(err)
		return
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Println(err)
		return
	}
	defer ch.Close()

	q, err := ch.QueueDeclare(
		"task_queue", // name
		true,         // durable
		false,        // delete when unused
		false,        // exclusive
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		log.Println(err)
		return
	}

	err = ch.Qos(
		1,     // prefetch count
		0,     // prefetch size
		false, // global
	)
	if err != nil {
		log.Println(err)
		return
	}
	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		false,  // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		log.Println(err)
		return
	}

	forever := make(chan bool)

	go func() {
		for d := range msgs {
			task := &pb.Task{}
			if err := proto.Unmarshal(d.Body, task); err != nil {
				log.Fatalln("Failed to parse:", err)
			}
			log.Println(task)
			log.Println("Task Received: ", task.VideoName)
			
			if (processVideo(ctx, task.VideoName)) {
				log.Printf("Done")
				d.Ack(false)
			} else {
				// requeue
				d.Nack(false, true)
			}
		}
	}()
	<- forever
}