FROM golang:1.4

RUN apt-get update && apt-get install -y upx

RUN go get \
		github.com/pwaller/goupx \
		github.com/codegangsta/cli \
		github.com/skelterjohn/rerun \
		github.com/gorilla/websocket

# Turn off cgo so that we end up with totally static binaries
ENV CGO_ENABLED 0


RUN go install -a -installsuffix=static std

COPY . /go/src/github.com/scraperwiki/hookbot

RUN go install \
	-installsuffix=static \
	-v github.com/scraperwiki/hookbot

RUN goupx /go/bin/hookbot

EXPOSE 8080

USER nobody:nogroup
ENTRYPOINT ["hookbot"]
