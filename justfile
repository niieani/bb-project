build:
	go build -o bb ./cmd/bb

test:
	go test ./...

docs-cli:
	go run ./cmd/bb-docs
