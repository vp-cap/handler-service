ARG SERVICE_PATH="/go/src/vp-cap/handler-service"

################## 1st Build Stage ####################
FROM golang:1.15 AS builder
LABEL stage=builder
ARG SERVICE_PATH
ARG GIT_USER
ARG GIT_PASS

WORKDIR ${SERVICE_PATH}
COPY go.mod .
COPY go.sum .

ENV GO111MODULE=on
RUN git config --global url."https://$GIT_USER:$GIT_PASS@github.com".insteadOf "https://github.com"
RUN go env -w GOPRIVATE=github.com/vp-cap
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go install
# RUN ls

# ################## 2nd Build Stage ####################
FROM tensorflow/tensorflow:1.15.0-py3
ARG SERVICE_PATH

COPY --from=builder ${SERVICE_PATH}/requirements.txt .

# Install dependencies
RUN pip3 install -r requirements.txt

RUN apt-get update ##[edited]
RUN apt-get install ffmpeg libsm6 libxext6  -y

COPY --from=builder /go/bin/handler-service /usr/local/bin/handler-service
COPY --from=builder ${SERVICE_PATH}/config.yaml /usr/local/bin/config.yaml
COPY --from=builder ${SERVICE_PATH}/process_video.py /usr/local/bin/process_video.py
COPY --from=builder ${SERVICE_PATH}/video.mp4 /usr/local/bin/video.mp4
COPY --from=builder ${SERVICE_PATH}/genproto/ /usr/local/bin/genproto/
COPY --from=builder ${SERVICE_PATH}/resnet50_coco_best_v2.0.1.h5 /usr/local/bin/resnet50_coco_best_v2.0.1.h5

RUN cd /usr/local/bin && ls


ENTRYPOINT ["./usr/local/bin/handler-service"]
