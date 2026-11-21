.PHONY: build test
API_URL ?= https://animewatcherbot-izrakwm6rq-ew.a.run.app
VERSION ?= dev

build:
	mkdir -p bin
	go build -trimpath -ldflags "-X main.defaultAPIURL=$(API_URL) -X main.version=$(VERSION)" -o bin/anime .

test:
	go test ./...
	PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests
	lua tests/test_skip_intro.lua
