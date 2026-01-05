.PHONY: help install-tools dev run list-devices fmt tidy env cert lp-group stop

help:
	@echo "make install-tools # go install github.com/air-verse/air@latest into ./bin"
	@echo "make dev           # start with air if available, otherwise go run"
	@echo "make run           # go run main.go"
	@echo "make list-devices  # ls /dev/usb and printer nodes"
	@echo "make lp-group      # add current user to lp group"
	@echo "make stop          # kill server using ECHO_PORT (default 1323)"
	@echo "make cert          # generate self-signed TLS cert (server.crt/key)"
	@echo "make fmt           # gofmt all go files"
	@echo "make tidy          # go mod tidy"
	@echo "make env           # show .env if present"

install-tools:
	GOBIN=$(PWD)/bin go install github.com/air-verse/air@latest


dev:
	@if command -v ./bin/air >/dev/null 2>&1; then \
		./bin/air .air.toml; \
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

lp-group:
	sudo usermod -aG lp $$USER

stop:
	@PORT=$${ECHO_PORT:-1323}; \
	PIDS=$$(lsof -ti tcp:$$PORT 2>/dev/null || true); \
	if [ -z "$$PIDS" ]; then \
		echo "no process found on :$$PORT"; \
	else \
		echo "killing $$PIDS on :$$PORT"; \
		kill $$PIDS; \
	fi
