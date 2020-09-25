#!/bin/bash -e

set -e 

PROTODIR=../proto

mkdir -p genproto
# python -m grpc_tools.protoc -I$PROTODIR --python_out=./genproto/ --grpc_python_out=./genproto/ $PROTODIR
protoc --go_out=plugins=grpc:genproto -I $PROTODIR $PROTODIR/task.proto
