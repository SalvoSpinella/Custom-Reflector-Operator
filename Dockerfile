# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /workspace

# Copy Go module files
COPY go.mod ./
COPY go.sum* ./
RUN go mod download || true

# Copy Go source files
COPY main.go main.go
COPY controllers/ controllers/

# Ensure dependencies are tidied inside Docker builder stage
RUN go mod tidy

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o custom-reflector main.go

# Run stage
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/custom-reflector /custom-reflector
USER 65532:65532

ENTRYPOINT ["/custom-reflector"]
