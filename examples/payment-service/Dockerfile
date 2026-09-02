# Build stage
FROM golang:1.27-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

# Build the payment service for Linux.
RUN CGO_ENABLED=0 GOOS=linux go build -o /payment-service ./examples/payment-service

# Runtime stage
FROM alpine:3.19

COPY --from=builder /payment-service /payment-service

EXPOSE 8080

ENTRYPOINT ["/payment-service"]
