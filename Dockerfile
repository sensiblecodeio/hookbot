FROM golang:1.12.1-alpine
ENV CGO_ENABLED=0
RUN go install -v net/http net/http/pprof

COPY ./vendor /go/src/github.com/sensiblecodeio/hookbot/vendor/
RUN go install -v github.com/sensiblecodeio/hookbot/vendor/...

COPY . /go/src/github.com/sensiblecodeio/hookbot

RUN go install \
	-v github.com/sensiblecodeio/hookbot

EXPOSE 8080

USER nobody:nogroup
ENTRYPOINT ["hookbot"]
