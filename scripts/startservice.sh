#!/usr/bin/env bash

set -euo pipefail

echo "Starting the gateway and backend services..."

echo "Stopping existing services..."
docker compose down

echo "Building images without cache..."
docker compose build --no-cache

echo "Starting services..."
docker compose up -d

echo "Service status:"
docker compose ps

echo "Service is up and running!"