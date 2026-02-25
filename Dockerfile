# ============================================
# Flight Log App - Multi-stage Docker Build
# ============================================

# Build stage
FROM golang:1.23-alpine AS build
WORKDIR /app

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary (static, no CGO)
RUN CGO_ENABLED=0 GOOS=linux go build -o flight-log-app .

# Runtime stage
FROM alpine:3.20
WORKDIR /app

# Install ca-certificates for HTTPS (Cosmos DB)
RUN apk --no-cache add ca-certificates

# Copy binary and static assets
COPY --from=build /app/flight-log-app .
COPY --from=build /app/static ./static

# Create shared upload directory
RUN mkdir -p /tmp/shared

EXPOSE 8080

CMD ["./flight-log-app"]
