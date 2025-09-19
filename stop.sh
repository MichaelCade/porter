#!/bin/bash

echo "🛑 Stopping any running Porter containers..."
# Stop by name filter
docker ps -q --filter "name=porter" | xargs -r docker stop
docker ps -a -q --filter "name=porter" | xargs -r docker rm

# Also stop by image name
docker ps -q --filter "ancestor=porter" | xargs -r docker stop
docker ps -a -q --filter "ancestor=porter" | xargs -r docker rm

# Check if anything is still using port 8080
CONTAINER_USING_PORT=$(docker ps -q --filter "publish=8080")
if [ ! -z "$CONTAINER_USING_PORT" ]; then
  echo "Force stopping container using port 8080: $CONTAINER_USING_PORT"
  docker stop $CONTAINER_USING_PORT
  docker rm $CONTAINER_USING_PORT
fi

echo "✅ Done."
