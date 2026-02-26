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

start-dolt:
  dolt sql-server --host 127.0.0.1 --port 3307 --data-dir .beads/dolt

start-dolt-bg:
  mkdir -p temp.local
  sh -c 'if ! command -v tmux >/dev/null 2>&1; then echo "tmux required for start-dolt-bg"; exit 1; fi'
  sh -c 'if tmux has-session -t dolt-bb-project 2>/dev/null; then echo "dolt already running (tmux:dolt-bb-project)"; exit 0; fi'
  tmux new-session -d -s dolt-bb-project "cd \"$(pwd)\" && dolt sql-server --host 127.0.0.1 --port 3307 --data-dir .beads/dolt > temp.local/dolt.log 2>&1"
  sleep 1
  lsof -nP -iTCP:3307 -sTCP:LISTEN

stop-dolt:
  sh -c 'if tmux has-session -t dolt-bb-project 2>/dev/null; then tmux kill-session -t dolt-bb-project && echo "stopped dolt (tmux:dolt-bb-project)"; else echo "no dolt tmux session"; fi'

dolt-logs:
  tail -f temp.local/dolt.log
