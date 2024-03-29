import sys
import os
import collections, functools, operator 
import genproto.models_pb2 as models 
import json
from imageai.Detection import VideoObjectDetection

# constants
VIDEO_OUT_PATH = "video_out"
VIDEO_INFERENCE_PATH = "video_inf"
SAVE_VIDEO = True
FPS = 30
MIN_DETECTION_PROB = 80
MIN_DURATION_AD_SECONDS = 5

ignore_list = ['person']

# def test():
# TODO come up with better algorithm to do this
def forFull(output_arrays, count_arrays, average_output_count):
    # remove unwanted objects
    count_arrays = [{k: v for k, v in d.items() if k not in ignore_list} for d in count_arrays]
    # print(len(count_arrays))
    global objectCountsEachSecond
    objectCountsEachSecond = count_arrays

    # Get average frequency for all the objects detected.
    global objectsToAvgFrequency
    objectsToAvgFrequency = {}
    reduced_object_count = dict(functools.reduce(operator.add, map(collections.Counter, count_arrays)))
    total = sum(reduced_object_count.values())
    for k, v in reduced_object_count.items():
        objectsToAvgFrequency[k] = v / total
    # print(objectsToAvgFrequency)

    # Get the top 5 objects
    global topFiveObjectsToAvgFrequency
    topFiveObjectsToAvgFrequency = dict(sorted(objectsToAvgFrequency.items(), key=operator.itemgetter(1), reverse=True)[:5])
    topFiveObjects = list(topFiveObjectsToAvgFrequency.keys())
    # print(topFiveObjectsToAvgFrequency)

    # Consider only the top 5 objects
    count_arrays = [{k: v for k, v in d.items() if k in topFiveObjects} for d in count_arrays]
    # print(count_arrays)
    i = 0
    curr = ''
    prev_index = 0
    global topFiveObjectsToInterval
    topFiveObjectsToInterval = {}
    for item in count_arrays:
        i += 1
        if curr != '' and (i - prev_index + 1 < MIN_DURATION_AD_SECONDS * FPS):
            topFiveObjectsToInterval[curr] = models.Interval(start=prev_index, end=i - 1)
            continue
        if not item:
            if curr != '' and (curr not in topFiveObjectsToInterval or i - prev_index -1 > topFiveObjectsToInterval[curr].end - topFiveObjectsToInterval[curr].start):
                topFiveObjectsToInterval[curr] = models.Interval(start=prev_index, end=i - 1)
            curr = ''
            prev_index = i
            continue
        max_value = max(item.values())
        max_object = [k for k,v in item.items() if v == max_value]
        # print(max_object)

        if curr not in max_object:
            if curr != '' and (curr not in topFiveObjectsToInterval or i - prev_index -1 > topFiveObjectsToInterval[curr].end - topFiveObjectsToInterval[curr].start):
                topFiveObjectsToInterval[curr] = models.Interval(start=prev_index, end=i - 1)
            curr = max_object[0]
            # print(curr)
            prev_index = i


# process video using imageAi lib and model
def process(video_path):

    execution_path = os.getcwd()

    detector = VideoObjectDetection()
    detector.setModelTypeAsRetinaNet()
    detector.setModelPath("./resnet50_coco_best_v2.1.0.h5")
    detector.loadModel()

    detector.detectObjectsFromVideo(
        input_file_path=os.path.join(execution_path, video_path),
        output_file_path=os.path.join(execution_path, VIDEO_OUT_PATH),
        save_detected_video=SAVE_VIDEO,
        frames_per_second=FPS,
        # per_second_function=forSeconds,
        video_complete_function=forFull,
        minimum_percentage_probability=MIN_DETECTION_PROB
    )

def main():
    if len(sys.argv) != 2:
        print('Invalid no of arguments')
        return
    print(sys.argv[1])
    process(sys.argv[1])
    # test()

    video_inference = models.VideoInference(objectCountsEachSecond=json.dumps(objectCountsEachSecond), objectsToAvgFrequency=objectsToAvgFrequency,
        topFiveObjectsToInterval=topFiveObjectsToInterval, topFiveObjectsToAvgFrequency=topFiveObjectsToAvgFrequency)
    # print(video_inference)

    f = open(VIDEO_INFERENCE_PATH, "wb")
    f.write(video_inference.SerializeToString())
    f.close()

if __name__ == "__main__":
    main()