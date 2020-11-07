#!/bin/bash -e

set -e 

PROTODIR=../proto

mkdir -p genproto
protoc --python_out=genproto -I $PROTODIR $PROTODIR/models.proto
mkdir -p genproto/models
protoc --go_out=genproto/models -I $PROTODIR $PROTODIR/models.proto
mkdir -p genproto/task
protoc --go_out=plugins=grpc:genproto/task -I $PROTODIR $PROTODIR/task.proto
