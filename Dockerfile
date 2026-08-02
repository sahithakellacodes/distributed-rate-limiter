# Shared base image for building the Go application
FROM golang:1.26.5-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Build the gateway binary
FROM builder AS gateway-builder
RUN go build -o gateway ./cmd/gateway/main.go

# Build the backend binary
FROM builder AS backend-builder
RUN go build -o backend ./cmd/backend/main.go

# Final image for the gateway
FROM alpine:latest AS gateway
WORKDIR /app
COPY --from=gateway-builder /app/gateway .
COPY internal/config/config.yaml ./internal/config/config.yaml

EXPOSE 8080
CMD ["./gateway"]

# Final image for the backend
FROM alpine:latest AS backend
WORKDIR /app
COPY --from=backend-builder /app/backend .
EXPOSE 8081
CMD ["./backend"]