APP := composepilot
DIST_DIR := dist
MAIN_PKG := ./cmd/composepilot
GO ?= go
VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build clean dist dist-linux dist-windows dist-darwin \
	dist-linux-amd64 dist-linux-arm64 dist-windows-amd64 dist-darwin-amd64 dist-darwin-arm64

build:
	$(GO) build -o ./tmp/$(APP) $(MAIN_PKG)

clean:
	rm -rf $(DIST_DIR)

dist: dist-linux dist-windows dist-darwin

dist-linux: dist-linux-amd64 dist-linux-arm64

dist-windows: dist-windows-amd64

dist-darwin: dist-darwin-amd64 dist-darwin-arm64

dist-linux-amd64:
	mkdir -p $(DIST_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)-linux-amd64 $(MAIN_PKG)

dist-linux-arm64:
	mkdir -p $(DIST_DIR)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)-linux-arm64 $(MAIN_PKG)

dist-windows-amd64:
	mkdir -p $(DIST_DIR)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)-windows-amd64.exe $(MAIN_PKG)

dist-darwin-amd64:
	mkdir -p $(DIST_DIR)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)-darwin-amd64 $(MAIN_PKG)

dist-darwin-arm64:
	mkdir -p $(DIST_DIR)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)-darwin-arm64 $(MAIN_PKG)
