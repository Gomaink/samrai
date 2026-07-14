#!/usr/bin/env sh
set -eu

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"

response="$(curl --fail --silent --show-error "$BASE_URL/health")"
printf '%s\n' "$response"
printf '%s' "$response" | grep -q '"status":"ok"'
printf '%s' "$response" | grep -q '"database":"ok"'
