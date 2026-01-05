.PHONY: help install-tools dev run list-devices fmt tidy env cert lp-group stop install-service

SERVICE_NAME := thermalprinter-go
SERVICE_FILE := $(SERVICE_NAME).service
SERVICE_UNIT_PATH := /etc/systemd/system/$(SERVICE_FILE)
BIN_TARGET := $(CURDIR)/bin/thermalprinter_go
INSTALL_BIN := /usr/local/bin/thermalprinter_go

	help:
	@echo "make install-tools # go install github.com/air-verse/air@latest into ./bin"
	@echo "make dev           # start with air if available, otherwise go run"
	@echo "make run           # go run main.go"
	@echo "make list-devices  # ls /dev/usb and printer nodes"
	@echo "make install-service # build and (re)register systemd service"
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

install-service: $(SERVICE_FILE)
	mkdir -p $(dir $(BIN_TARGET))
	go build -o $(BIN_TARGET) main.go
	sudo install -Dm755 $(BIN_TARGET) $(INSTALL_BIN)
	sudo install -Dm644 $(SERVICE_FILE) $(SERVICE_UNIT_PATH)
	sudo systemctl daemon-reload
	sudo systemctl enable $(SERVICE_NAME)
	sudo systemctl restart $(SERVICE_NAME)
