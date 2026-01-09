FROM golang:1.25-alpine AS build-stage

RUN apk update && apk upgrade

WORKDIR /app
COPY . .
RUN go mod download

WORKDIR /app/cmd
RUN GOOS=linux go build -o stickerbot main.go

FROM alpine:latest AS release-stage

RUN apk add imagemagick ffmpeg

WORKDIR /app
COPY --from=build-stage /app/cmd/stickerbot .
RUN chmod +x stickerbot

ENTRYPOINT ["./stickerbot"]