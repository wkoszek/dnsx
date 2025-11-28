.PHONY: build clean install test fmt vet release all

BINARY=dnsx

all: build

build:
	go build -o $(BINARY) ./cmd/dnsx

clean:
	rm -f $(BINARY)
	rm -rf dist/

install: build
	mkdir -p $(HOME)/bin
	cp $(BINARY) $(HOME)/bin/

test:
	go test -v ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

release: clean
	mkdir -p dist
	GOOS=darwin GOARCH=amd64 go build -o dist/$(BINARY)-darwin-amd64 ./cmd/dnsx
	GOOS=darwin GOARCH=arm64 go build -o dist/$(BINARY)-darwin-arm64 ./cmd/dnsx
	GOOS=linux GOARCH=amd64 go build -o dist/$(BINARY)-linux-amd64 ./cmd/dnsx
	GOOS=linux GOARCH=arm64 go build -o dist/$(BINARY)-linux-arm64 ./cmd/dnsx
	GOOS=windows GOARCH=amd64 go build -o dist/$(BINARY)-windows-amd64.exe ./cmd/dnsx
	ls -lh dist/
