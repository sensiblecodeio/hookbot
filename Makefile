all: hookbot

hookbot: Dockerfile
	git submodule update --init
	docker build -t sensiblecodeio/hookbot .
	docker create --name hookbot-tmp sensiblecodeio/hookbot
	docker cp hookbot-tmp:/go/bin/hookbot .
	docker rm hookbot-tmp
	chmod +x ./hookbot

# GNU Make instructions
.PHONY:
# Required for hanoverd.deps
.DELETE_ON_ERROR:
