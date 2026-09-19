FROM golang:1.27.1-alpine3.24 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux \
    go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /airo \
    ./cmd/main.go

FROM alpine:3.24

RUN apk add --no-cache ca-certificates \
    && addgroup -S airo \
    && adduser -S -G airo airo

COPY --from=builder /airo /airo

USER airo

EXPOSE 8081

ENTRYPOINT ["/airo"]