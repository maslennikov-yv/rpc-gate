#!/bin/bash

# Setup script to download dependencies
echo "Downloading Go dependencies..."

# Download all dependencies
go mod download

# Tidy up go.mod and go.sum
go mod tidy

# Verify dependencies
go mod verify

echo "Dependencies setup complete!"
