#!/bin/sh
# OpenCode launcher for the backend template (the harness-config's `command.base` runs
# `sh /home/scion/.local/bin/opencode-launch.sh`; this file ships in the template's home/ overlay).
#
# Model pin (lessons-learned.md L3): OpenCode's models.dev catalog marks gemini-2.5 as deprecated, so with the full
# catalog OpenCode resolves "Gemini 2.5 Pro" to an image/TTS or unavailable variant and every call fails with 404.
# The Scion provisioner pre-fetches the full catalog into ~/.cache/opencode/models.json, and OpenCode refreshes that
# file in the background, so a pin written once doesn't survive restarts. This launcher rewrites the pinned
# catalog on every start, after the provisioner and before OpenCode reads it. (The stock launcher deleted the file,
# which makes OpenCode fall back to its built-in snapshot.)
export GOOGLE_CLOUD_PROJECT="${HUB_PROJECT}"
export GOOGLE_CLOUD_LOCATION=us-central1
export GOOGLE_CLOUD_REGION=us-central1
export GOOGLE_VERTEX_PROJECT="${HUB_PROJECT}"
export GOOGLE_VERTEX_LOCATION=us-central1
export VERTEXAI_PROJECT="${HUB_PROJECT}"
export VERTEX_LOCATION=us-central1

mkdir -p "$HOME/.config/opencode" "$HOME/.local/state/opencode" "$HOME/.cache/opencode"
rm -f "$HOME/.config/opencode/.opencode.json"

cat << 'JSON' > "$HOME/.cache/opencode/models.json"
{
  "google-vertex": {
    "id": "google-vertex",
    "name": "Vertex",
    "env": ["GOOGLE_CLOUD_PROJECT", "GOOGLE_CLOUD_LOCATION"],
    "npm": "@ai-sdk/google-vertex",
    "models": {
      "gemini-2.5-pro": {
        "id": "gemini-2.5-pro",
        "name": "Gemini 2.5 Pro",
        "tool_call": true,
        "reasoning": true,
        "attachment": true,
        "temperature": true,
        "cost": {"input": 1.25, "output": 10},
        "limit": {"context": 1048576, "output": 65536}
      },
      "gemini-2.5-flash": {
        "id": "gemini-2.5-flash",
        "name": "Gemini 2.5 Flash",
        "tool_call": true,
        "reasoning": true,
        "attachment": true,
        "temperature": true,
        "cost": {"input": 0.15, "output": 0.6},
        "limit": {"context": 1048576, "output": 65536}
      }
    }
  }
}
JSON

cat << 'JSON' > "$HOME/.config/opencode/opencode.json"
{
  "$schema": "https://opencode.ai/config.json",
  "model": "google-vertex/gemini-2.5-pro",
  "small_model": "google-vertex/gemini-2.5-flash",
  "disabled_providers": ["google-vertex-anthropic", "opencode"]
}
JSON

cat << 'JSON' > "$HOME/.local/state/opencode/model.json"
{"recent": [{"providerID": "google-vertex", "modelID": "gemini-2.5-pro"}], "favorite": [], "variant": {}}
JSON

# OpenCode would treat GITHUB_TOKEN as a GitHub Copilot credential; Scion's git credential helper reads
# SCION_GIT_TOKEN instead, so git clone/push keep working.
if [ -n "${GITHUB_TOKEN:-}" ]; then
  export SCION_GIT_TOKEN="$GITHUB_TOKEN"
  unset GITHUB_TOKEN
fi
exec opencode --auto -m google-vertex/gemini-2.5-pro "$@"
