.PHONY: all build clean test server client

all: build

build: server client

server:
	@echo "Building server..."
	cd server && go build -o cmd/server/server ./cmd/server

client:
	@echo "Building client..."
	cd client && go build -o cmd/client/client ./cmd/client

clean:
	@echo "Cleaning..."
	rm -f server/cmd/server/server
	rm -f client/cmd/client/client

test:
	@echo "Running tests..."
	cd server && go test ./...
	cd client && go test ./...
	cd shared && go test ./...

deps:
	@echo "Downloading dependencies..."
	cd server && go mod download
	cd client && go mod download
	cd shared && go mod download

tidy:
	@echo "Tidying modules..."
	cd server && go mod tidy
	cd client && go mod tidy
	cd shared && go mod tidy

run-server:
	@echo "Running server..."
	cd server/cmd/server && go run .

run-client:
	@echo "Running client..."
	cd client/cmd/client && go run .
