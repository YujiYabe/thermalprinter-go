.PHONY: help install-air dev run list-devices fmt tidy env cert

help:
	@echo "make install-air   # go install github.com/air-verse/air@latest"
	@echo "make dev           # start with air if available, otherwise go run"
	@echo "make run           # go run main.go"
	@echo "make list-devices  # ls /dev/usb and printer nodes"
	@echo "make cert          # generate self-signed TLS cert (server.crt/key)"
	@echo "make fmt           # gofmt all go files"
	@echo "make tidy          # go mod tidy"
	@echo "make env           # show .env if present"

install-air:
	GO111MODULE=on go install github.com/air-verse/air@latest

dev:
	@if command -v air >/dev/null 2>&1; then \
		air; \
	else \
		echo "air not found; falling back to go run"; \
		go run main.go; \
	fi

run:
	go run main.go

cert:
	openssl req -x509 -newkey rsa:2048 -nodes -keyout server.key -out server.crt -days 365 -subj "/CN=localhost"

list-devices:
	@echo "Listing /dev/usb (and lp* nodes if present)"
	@ls -la /dev/usb || true
	@ls -la /dev/usb/lp* 2>/dev/null || true

fmt:
	gofmt -w $(shell find . -name '*.go' -not -path './vendor/*')

tidy:
	go mod tidy

env:
	@if [ -f .env ]; then cat .env; else echo ".env not found"; fi
