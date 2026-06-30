#!/usr/bin/env bash
# bunny-stats.sh —— 定时拉取 Bunny CDN 累计请求数，写入 stats/bunny.count 供 go 的 /stats 读取。
#
# 架构：systemd timer 每小时跑本脚本 → 调 Bunny statistics API → 写纯数字文件。
# go 不再自己调 API（避免请求量大时触发 API 限流），只读 bunny.count。
#
# 凭证从 /opt/gravatar-proxy/.env 读取（systemd EnvironmentFile 格式）：
#   BUNNY_API_KEY       Bunny 账户 API Key
#   BUNNY_PULLZONE_ID   gravatar 的 Pull Zone ID（如 6086222）
#   BUNNY_ZONE_START    Pull Zone 创建日（YYYY-MM-DD，用于拉累计总量）
#
# 部署位置：/opt/gravatar-proxy/stats/bunny-stats.sh（属 caddy，700：含只读用的 key）

set -euo pipefail

ENV_FILE="${BUNNY_ENV_FILE:-/opt/gravatar-proxy/.env}"
OUT_FILE="${BUNNY_COUNT_FILE:-/opt/gravatar-proxy/stats/bunny.count}"
LOG_TAG="bunny-stats"

# 加载凭证（.env 是 KEY=VALUE 格式）
if [ -f "$ENV_FILE" ]; then
  set -a
  # shellcheck disable=SC1090
  . "$ENV_FILE"
  set +a
fi

if [ -z "${BUNNY_API_KEY:-}" ] || [ -z "${BUNNY_PULLZONE_ID:-}" ]; then
  logger -t "$LOG_TAG" "缺少 BUNNY_API_KEY 或 BUNNY_PULLZONE_ID，跳过"
  exit 0
fi

START="${BUNNY_ZONE_START:-2026-06-07}"
END="$(date -u +%Y-%m-%d)"
URL="https://api.bunny.net/statistics?pullZone=${BUNNY_PULLZONE_ID}&dateFrom=${START}&dateTo=${END}&serverZoneId=-1"

# 调 API 取累计请求数；失败则保留旧值（不覆盖）
RESP="$(curl -fsS --max-time 20 -H "AccessKey: ${BUNNY_API_KEY}" "$URL" 2>/dev/null)" || {
  logger -t "$LOG_TAG" "Bunny API 调用失败，保留旧值"
  exit 0
}

# 提取 TotalRequestsServed 字段（兼容 python3/jq，优先 jq）
if command -v jq >/dev/null 2>&1; then
  COUNT="$(printf '%s' "$RESP" | jq -r '.TotalRequestsServed // empty')"
else
  COUNT="$(printf '%s' "$RESP" | python3 -c 'import sys,json; print(json.load(sys.stdin).get("TotalRequestsServed",""))' 2>/dev/null || true)"
fi

if [ -z "$COUNT" ] || [ "$COUNT" = "null" ]; then
  logger -t "$LOG_TAG" "解析 TotalRequestsServed 失败，保留旧值"
  exit 0
fi

# 原子写入（临时文件 + rename，go 读取时不会读到半截）
TMP="${OUT_FILE}.tmp"
printf '%s' "$COUNT" > "$TMP"
mv -f "$TMP" "$OUT_FILE"
logger -t "$LOG_TAG" "已更新 ${OUT_FILE} = ${COUNT}（区间 ${START}~${END}）"
