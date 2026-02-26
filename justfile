build:
	go build -o bb ./cmd/bb

install-dev link_dir="$HOME/bin":
  just build
	mkdir -p {{ link_dir }}
	ln -sfn "$(pwd)/bb" "{{ link_dir }}/bb"
	@echo "linked {{ link_dir }}/bb -> $(pwd)/bb"

uninstall-dev link_dir="$HOME/bin":
	rm -f "{{ link_dir }}/bb"

test:
	go test ./...

docs-cli:
	go run ./cmd/bb-docs
