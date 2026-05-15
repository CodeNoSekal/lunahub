APP=lunahub

.PHONY: build install fmt test

build:
	go build -o bin/$(APP) ./cmd/lunahub

install:
	go build -o /usr/local/bin/$(APP) ./cmd/lunahub
	chmod 755 /usr/local/bin/$(APP)

fmt:
	gofmt -w ./cmd ./internal 2>/dev/null || gofmt -w ./cmd

test:
	go test ./...
