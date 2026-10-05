#!/usr/bin/env bash
#
# Temporarily allow a domain for one group, then automatically revert.
#
# pab has no built-in TTL/scheduling for allow-list entries and no command to
# edit a group's allowedDomains/blockedDomains at all (only `pab map`/`unmap`,
# which touch networkGroupMap, not group domain lists). This script is the
# manual workaround: it edits dnsApp.config directly, shells out to `pab
# deploy` the same way a human would, then schedules its own revert as a
# detached background process so the allow doesn't outlive --minutes.
#
# Usage:
#   temp-allow-domain.sh --domain <domain> --group <group> [--minutes <N>] [--config <path>] [--pab <path>]
#   temp-allow-domain.sh --list
#   temp-allow-domain.sh --cancel --domain <domain> --group <group>

set -euo pipefail

STATE_DIR="${PAB_TEMP_ALLOW_STATE_DIR:-$HOME/.local/state/pab-temp-allow}"

usage() {
  cat >&2 <<'EOF'
Usage:
  temp-allow-domain.sh --domain <domain> --group <group> [--minutes <N>] [--config <path>] [--pab <path>]
  temp-allow-domain.sh --list
  temp-allow-domain.sh --cancel --domain <domain> --group <group>

Adds --domain to --group's allowedDomains in dnsApp.config, deploys it with
pab, and schedules an automatic revert + redeploy after --minutes (default 60).

Options:
  --domain   Domain to temporarily allow (required for apply/cancel)
  --group    Group name in dnsApp.config to modify (required for apply/cancel)
  --minutes  Minutes until auto-revert (default: 60)
  --config   Path to dnsApp.config (default: ./dnsApp.config)
  --pab      Path to the pab binary (default: ./pab)
  --list     List currently scheduled temporary allows
  --cancel   Revert immediately and cancel the scheduled auto-revert

Caveats:
  - The scheduled revert is a background shell process tied to this host; it
    does not survive a reboot. Use --list to check on it later, or re-run
    with --cancel if you need to revert before it fires.
  - Requires jq and a configured pab (TECHNITIUM_URL/TECHNITIUM_TOKEN or
    ~/.config/pab/secrets.json) to actually deploy.
EOF
  exit 1
}

MODE="apply"
DOMAIN=""
GROUP=""
MINUTES=60
CONFIG_PATH="${PAB_CONFIG:-dnsApp.config}"
PAB_BIN="${PAB_BIN:-./pab}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain) DOMAIN="$2"; shift 2 ;;
    --group) GROUP="$2"; shift 2 ;;
    --minutes) MINUTES="$2"; shift 2 ;;
    --config) CONFIG_PATH="$2"; shift 2 ;;
    --pab) PAB_BIN="$2"; shift 2 ;;
    --list) MODE="list"; shift ;;
    --cancel) MODE="cancel"; shift ;;
    --_revert-worker) MODE="revert-worker"; STATE_FILE="$2"; shift 2 ;;
    -h|--help) usage ;;
    *) usage ;;
  esac
done

mkdir -p "$STATE_DIR"

state_file_for() {
  local group="$1" domain="$2"
  local safe_group="${group// /_}"
  echo "$STATE_DIR/${safe_group}__${domain}.json"
}

require_jq() {
  command -v jq >/dev/null || { echo "jq is required but not found in PATH" >&2; exit 1; }
}

log() { echo "[$(date -Is)] $*"; }

# --- --list ---------------------------------------------------------------
if [[ "$MODE" == "list" ]]; then
  require_jq
  shopt -s nullglob
  files=("$STATE_DIR"/*.json)
  shopt -u nullglob
  if [[ ${#files[@]} -eq 0 ]]; then
    echo "No scheduled temporary allows."
    exit 0
  fi
  for f in "${files[@]}"; do
    domain=$(jq -r .domain "$f")
    group=$(jq -r .group "$f")
    revert_at=$(jq -r .revert_at "$f")
    pid=$(jq -r .pid "$f")
    alive="dead"
    kill -0 "$pid" 2>/dev/null && alive="running"
    echo "$domain in $group -> reverts at $(date -d "@$revert_at" '+%Y-%m-%d %H:%M:%S') (worker pid $pid: $alive)"
  done
  exit 0
fi

if [[ "$MODE" != "revert-worker" ]]; then
  [[ -z "$DOMAIN" || -z "$GROUP" ]] && usage
  STATE_FILE="${STATE_FILE:-$(state_file_for "$GROUP" "$DOMAIN")}"
fi

revert_now() {
  require_jq
  if [[ ! -f "$STATE_FILE" ]]; then
    log "no scheduled revert found at $STATE_FILE (already reverted or never applied by this script)"
    return 0
  fi

  local domain group config pab was_already_allowed
  domain=$(jq -r .domain "$STATE_FILE")
  group=$(jq -r .group "$STATE_FILE")
  config=$(jq -r .config "$STATE_FILE")
  pab=$(jq -r .pab "$STATE_FILE")
  was_already_allowed=$(jq -r .was_already_allowed "$STATE_FILE")

  if [[ "$was_already_allowed" == "true" ]]; then
    log "$domain was already in $group's allowedDomains before this script ran; leaving it in place"
  else
    log "removing $domain from $group's allowedDomains in $config"
    jq --arg g "$group" --arg d "$domain" \
      '.groups[$g].allowedDomains = ((.groups[$g].allowedDomains // []) - [$d])' \
      "$config" > "${config}.tmp"
    mv "${config}.tmp" "$config"

    log "redeploying $config via $pab"
    "$pab" deploy -f --config "$config"
  fi

  rm -f "$STATE_FILE"
  log "revert complete for $domain in $group"
}

# --- --cancel ---------------------------------------------------------------
if [[ "$MODE" == "cancel" ]]; then
  require_jq
  if [[ -f "$STATE_FILE" ]]; then
    pid=$(jq -r .pid "$STATE_FILE")
    kill "$pid" 2>/dev/null || true
  fi
  revert_now
  exit 0
fi

# --- background worker (spawned by apply, not run directly) ----------------
if [[ "$MODE" == "revert-worker" ]]; then
  require_jq
  [[ -f "$STATE_FILE" ]] || exit 0
  revert_at=$(jq -r .revert_at "$STATE_FILE")
  now=$(date +%s)
  remaining=$(( revert_at - now ))
  (( remaining > 0 )) && sleep "$remaining"
  revert_now
  exit 0
fi

# --- apply -------------------------------------------------------------------
require_jq

[[ -f "$CONFIG_PATH" ]] || { echo "config not found: $CONFIG_PATH" >&2; exit 1; }
[[ -x "$PAB_BIN" ]] || { echo "pab binary not found/executable at $PAB_BIN (build it first)" >&2; exit 1; }

jq -e --arg g "$GROUP" '.groups[$g]' "$CONFIG_PATH" >/dev/null \
  || { echo "group not found in $CONFIG_PATH: $GROUP" >&2; exit 1; }

if [[ -f "$STATE_FILE" ]]; then
  echo "a temporary allow for $DOMAIN in $GROUP is already scheduled (see --list); --cancel it first" >&2
  exit 1
fi

was_already_allowed=$(jq -r --arg g "$GROUP" --arg d "$DOMAIN" \
  '(.groups[$g].allowedDomains // []) | index($d) != null' "$CONFIG_PATH")

if [[ "$was_already_allowed" != "true" ]]; then
  log "adding $DOMAIN to $GROUP's allowedDomains in $CONFIG_PATH"
  jq --arg g "$GROUP" --arg d "$DOMAIN" \
    '.groups[$g].allowedDomains = ((.groups[$g].allowedDomains // []) + [$d] | unique)' \
    "$CONFIG_PATH" > "${CONFIG_PATH}.tmp"
  mv "${CONFIG_PATH}.tmp" "$CONFIG_PATH"

  log "deploying $CONFIG_PATH via $PAB_BIN"
  "$PAB_BIN" deploy -f --config "$CONFIG_PATH"
else
  log "$DOMAIN is already in $GROUP's allowedDomains; deploying nothing, scheduling no-op revert"
fi

revert_at=$(( $(date +%s) + MINUTES * 60 ))
abs_config=$(readlink -f "$CONFIG_PATH")
abs_pab=$(readlink -f "$PAB_BIN")

jq -n \
  --arg domain "$DOMAIN" --arg group "$GROUP" --arg config "$abs_config" \
  --arg pab "$abs_pab" --argjson revert_at "$revert_at" \
  --argjson was_already_allowed "$was_already_allowed" \
  '{domain: $domain, group: $group, config: $config, pab: $pab, revert_at: $revert_at, was_already_allowed: $was_already_allowed, pid: 0}' \
  > "$STATE_FILE"

setsid nohup "$0" --_revert-worker "$STATE_FILE" >>"$STATE_DIR/worker.log" 2>&1 &
worker_pid=$!
disown

jq --argjson pid "$worker_pid" '.pid = $pid' "$STATE_FILE" > "${STATE_FILE}.tmp"
mv "${STATE_FILE}.tmp" "$STATE_FILE"

log "$DOMAIN allowed for $GROUP; will auto-revert at $(date -d "@$revert_at" '+%Y-%m-%d %H:%M:%S') (worker pid $worker_pid)"
