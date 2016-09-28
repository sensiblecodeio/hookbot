all: hookbot

# So that make knows about hookbot's other dependencies.
-include hookbot.deps

# Compute a file that looks like "hookbot: file.go" for all go source files
# that hookbot depends on.
hookbot.deps:
	./generate-deps.sh hookbot . > $@

hookbot: hookbot.deps Dockerfile
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
