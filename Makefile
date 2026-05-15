APP=lunahub

.PHONY: build fmt test clean

build:
	go build -o bin/$(APP) ./cmd/lunahub

fmt:
	gofmt -w ./cmd ./internal 2>/dev/null || gofmt -w ./cmd

test:
	go test ./...

clean:
	rm -rf bin
