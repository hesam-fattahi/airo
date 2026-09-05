# Build Stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy dependency manifests
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the AIRO operator static binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /airo cmd/main.go

# Runtime Stage
FROM alpine:3.19

COPY --from=builder /airo /airo

EXPOSE 8081

ENTRYPOINT ["/airo"]
