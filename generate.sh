#!/bin/bash

# Add Go bin to PATH
export PATH=$PATH:$(go env GOPATH)/bin

# Install protoc-gen-go and protoc-gen-go-grpc if not already installed
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Generate gRPC code from proto file
protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    proto/tts.proto

echo "gRPC code generated successfully!"
