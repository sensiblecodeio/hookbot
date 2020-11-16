FROM golang:1.15.5-alpine

RUN apk add git

USER nobody:nogroup

ENV CGO_ENABLED=0 GO111MODULE=on XDG_CACHE_HOME=/tmp/.cache

WORKDIR /go/src/github.com/sensiblecodeio/hookbot

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go install -v

EXPOSE 8080

ENTRYPOINT ["hookbot"]
