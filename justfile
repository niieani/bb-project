build:
	go build -o bb ./cmd/bb

build-dev:
	mkdir -p .dist/dev
	go build -o .dist/dev/bb ./cmd/bb

install-dev link_dir=".bin":
	mkdir -p {{ link_dir }}
	just build-dev
	ln -sfn "$(pwd)/.dist/dev/bb" "{{ link_dir }}/bb"
	@echo "linked {{ link_dir }}/bb -> $(pwd)/.dist/dev/bb"

uninstall-dev link_dir=".bin":
	rm -f "{{ link_dir }}/bb"

test:
	go test ./...

docs-cli:
	go run ./cmd/bb-docs
