#!/bin/bash

# Initialize dependencies script
echo "Initializing Go dependencies..."

# Remove any existing go.sum to start fresh
rm -f go.sum

# Initialize go.mod if it doesn't exist
if [ ! -f go.mod ]; then
    echo "Initializing go.mod..."
    go mod init streaming-server
fi

# Download dependencies one by one to avoid conflicts
echo "Downloading core dependencies..."

# Download Gorilla WebSocket
echo "Getting gorilla/websocket..."
go get github.com/gorilla/websocket@v1.5.1

# Download Kafka Go client
echo "Getting segmentio/kafka-go..."
go get github.com/segmentio/kafka-go@v0.4.47

# Download testing dependencies
echo "Getting stretchr/testify..."
go get github.com/stretchr/testify@v1.8.4

# Download UUID library
echo "Getting google/uuid..."
go get github.com/google/uuid@v1.4.0

# Clean up and verify
echo "Cleaning up dependencies..."
go mod tidy
go mod verify

echo "Dependencies initialized successfully!"
echo "Available modules:"
go list -m all
