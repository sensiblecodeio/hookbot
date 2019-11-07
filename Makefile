all: hookbot

hookbot: FORCE
	docker build -t sensiblecodeio/hookbot .
	docker create --name hookbot-tmp sensiblecodeio/hookbot
	docker cp hookbot-tmp:/go/bin/hookbot .
	docker rm hookbot-tmp
	chmod +x ./hookbot

test:
	go test -v ./...

# GNU Make instructions
.PHONY: FORCE
# Required for hanoverd.deps
.DELETE_ON_ERROR:
