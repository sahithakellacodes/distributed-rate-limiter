#!/usr/bin/env bash

set -e

echo "Starting local setup..."

set -a
source .env
set +a

cleanup() {
    echo "Stopping services..."
    kill "$BACKEND_PID" 2>/dev/null || true
}

trap cleanup EXIT

echo "Starting backend..."
go run ./cmd/backend/main.go &
BACKEND_PID=$!

echo "Starting gateway..."
go run ./cmd/gateway/main.go