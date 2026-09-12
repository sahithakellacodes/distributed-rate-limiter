#!/usr/bin/env bash

set -e

echo "Starting the gateway and backend services..."

docker compose down && docker compose up --build
