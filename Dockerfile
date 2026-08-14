FROM golang:1.26-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /aegis cmd/aegis/main.go

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

COPY --from=builder /aegis /usr/local/bin/aegis

ENTRYPOINT ["/usr/local/bin/aegis"]
CMD ["--config", "/config.yaml"]