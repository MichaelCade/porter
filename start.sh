#!/bin/bash

# Create data directories if they don't exist
mkdir -p ~/porter-data/extracted ~/porter-data/converted

# Stop any existing Porter containers
docker ps -q --filter "name=porter" | xargs -r docker stop

# Start the Porter application in the background
echo "🚀 Starting Porter..."
docker build -t porter .
docker run -d --name porter \
  -v ~/.aws:/root/.aws:ro \
  -v ~/.azure:/root/.azure:ro \
  -v ~/porter-data/extracted:/app/extracted \
  -v ~/porter-data/converted:/app/converted \
  -p 8080:8080 \
  porter

# Open browser to the application
echo "Opening Porter in your browser..."
sleep 2
open http://localhost:8080
