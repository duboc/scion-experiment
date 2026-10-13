#!/bin/sh
# Hermes launcher for the reviewer template (the harness-config's `command.base` runs
# `sh /home/scion/.local/bin/hermes-launch.sh`; this file ships in the template's home/ overlay).
# Hermes reads its provider and model from ~/.hermes/config.yaml, written here on every start. Hermes has no
# --prompt flag, so the initial task Scion passes is typed into the tmux pane once Hermes is up.
mkdir -p "$HOME/.hermes"
cat << 'YAML' > "$HOME/.hermes/config.yaml"
provider: vertex
model: google/gemini-2.5-pro
hooks_auto_accept: true
vertex:
  project_id: ${HUB_PROJECT}
  region: us-central1
YAML

TASK=""
while [ $# -gt 0 ]; do
  case "$1" in
    --prompt)
      shift
      TASK="${1:-}"
      ;;
    --continue)
      ;;
  esac
  shift 2>/dev/null || break
done

if [ -n "$TASK" ]; then
  (sleep 4 && tmux send-keys -t scion -l "$TASK" && tmux send-keys -t scion Enter) &
fi

exec hermes chat --yolo --accept-hooks --provider vertex -m google/gemini-2.5-pro
