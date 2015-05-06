hookbot: build
	@docker run --rm --entrypoint=cat hookbot /go/bin/hookbot > hookbot
	chmod +x hookbot

run: build
	-docker run --tty --interactive --rm \
		--publish=8080:8080 \
		--env=HOOKBOT_KEY=test \
		--env=HOOKBOT_GITHUB_SECRET=test \
		--name=hookbot \
		hookbot

build:
	@docker build -t hookbot .

.PHONY: run build
