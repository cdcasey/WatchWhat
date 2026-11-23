SHELL := /bin/bash
GO ?= go
SQLC_VERSION ?= v1.31.1
TAILWIND_VERSION ?= v4.3.5
TAILWIND := bin/tailwindcss
BINARY ?= bin/watchwhat

UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)
ifeq ($(UNAME_S),Darwin)
  TW_OS := macos
else
  TW_OS := linux
endif
ifneq (,$(filter $(UNAME_M),arm64 aarch64))
  TW_ARCH := arm64
else
  TW_ARCH := x64
endif

.PHONY: generate css sqlc build dev test clean

generate:
	$(GO) tool templ generate

$(TAILWIND):
	@mkdir -p bin
	curl -fsSL -o $@ https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/tailwindcss-$(TW_OS)-$(TW_ARCH)
	chmod +x $@

css: $(TAILWIND)
	$(TAILWIND) -i web/tailwind/input.css -o web/static/css/app.css --minify

sqlc:
	$(GO) run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate

build: generate css
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags="-s -w" -o $(BINARY) ./cmd/server

# Rebuilds CSS on change in the background, regenerates templ on change, restarts the server.
dev: generate css
	@set -a; [ -f .env ] && . ./.env; set +a; \
	$(TAILWIND) -i web/tailwind/input.css -o web/static/css/app.css --watch=always & \
	trap 'kill %1' EXIT; \
	$(GO) tool templ generate --watch --proxy=http://localhost:8080 --cmd="$(GO) run ./cmd/server"

test: generate
	$(GO) test ./...

clean:
	rm -rf bin web/static/css/app.css
	find . -name '*_templ.go' -delete
