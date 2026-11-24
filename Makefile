APP_NAME := sni-spoof
BUILD_DIR := bin

.PHONY: all build test clean cross-compile

all: test build

build:
	go build -o $(APP_NAME) ./cmd/sni-spoof

test:
	go test -v ./...

cross-compile:
	mkdir -p $(BUILD_DIR)
	# Linux amd64
	GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 ./cmd/sni-spoof
	# Linux arm64
	GOOS=linux GOARCH=arm64 go build -o $(BUILD_DIR)/$(APP_NAME)-linux-arm64 ./cmd/sni-spoof
	# macOS Apple Silicon (arm64)
	GOOS=darwin GOARCH=arm64 go build -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64 ./cmd/sni-spoof
	# macOS Intel (amd64)
	GOOS=darwin GOARCH=amd64 go build -o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64 ./cmd/sni-spoof
	# Windows amd64
	GOOS=windows GOARCH=amd64 go build -o $(BUILD_DIR)/$(APP_NAME)-windows-amd64.exe ./cmd/sni-spoof

clean:
	rm -f $(APP_NAME)
	rm -rf $(BUILD_DIR)

