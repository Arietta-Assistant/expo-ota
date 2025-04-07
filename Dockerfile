FROM node:18-alpine AS dashboard-builder
WORKDIR /app/dashboard
COPY dashboard/package.json dashboard/package-lock.json ./
RUN npm ci
COPY dashboard ./
RUN npm run build

FROM golang:1.24.2-alpine AS builder

# Install git and build dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download specific problematic dependencies first
RUN go get firebase.google.com/go/v4@v4.14.0 && \
    go get gorm.io/driver/postgres && \
    go get gorm.io/gorm && \
    go get google.golang.org/api/option

# Update dependencies and download them
RUN go mod tidy && go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o main ./cmd/api

FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/main .
COPY --from=builder /app/config ./config
COPY --from=builder /app/updates ./updates

# Expose port
EXPOSE 8080

# Run the application
CMD ["./main"]
