FROM golang:alpine

ARG command
COPY . /app
WORKDIR /app

ENV COMMAND=${command}
ENTRYPOINT go run . $COMMAND