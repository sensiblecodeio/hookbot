FROM golang:1.6

COPY ./vendor /go/src/github.com/scraperwiki/hookbot/vendor/
RUN go install -v github.com/scraperwiki/hookbot/vendor/...

COPY . /go/src/github.com/scraperwiki/hookbot

RUN go install \
	-v github.com/scraperwiki/hookbot

EXPOSE 8080

USER nobody:nogroup
ENTRYPOINT ["hookbot"]
