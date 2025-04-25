FROM node:18-alpine AS dashboard-builder
WORKDIR /app/dashboard
COPY dashboard/package.json dashboard/package-lock.json ./
RUN npm ci
COPY dashboard ./
RUN npm run build && \
    ls -la dist && \
    echo "Dashboard built successfully at $(date)"

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

# Copy dashboard files from builder
COPY --from=dashboard-builder /app/dashboard/dist /app/dashboard/dist

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o main ./cmd/api && \
    echo "Application built successfully at $(date)"

FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/main .
COPY --from=builder /app/config ./config
COPY --from=builder /app/updates ./updates
COPY --from=dashboard-builder /app/dashboard/dist /app/dashboard/dist

# Verify dashboard files
RUN ls -la /app/dashboard/dist || echo "Dashboard directory not found"

# Set environment variables
ENV USE_DASHBOARD=true
ENV PORT=3000

# Health check
HEALTHCHECK --interval=5s --timeout=3s --start-period=5s --retries=3 CMD wget -qO- http://localhost:3000/health || exit 1

# Expose port
EXPOSE 3000

# Run the application
CMD ["./main"]
