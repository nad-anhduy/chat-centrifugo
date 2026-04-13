# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

# Building app
RUN cd cmd/chat && go build -o server .

FROM alpine:3.18

RUN apk update && apk add --no-cache tzdata
COPY --from=builder /app/cmd/chat/server /app/
COPY ./config/config.yaml /app/config/config.yaml

ENV ENV_CONFIG_ONLY=true

WORKDIR /app

EXPOSE 8080
ENV GIN_MODE=release

# Run the web service on container startup.
CMD ["/app/server"]