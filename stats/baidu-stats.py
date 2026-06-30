#!/usr/bin/env python3
"""定时拉取百度 CDN 请求数，写入 baidu.count 供 Go 的 /stats 合并读取。

密钥从 /opt/gravatar-proxy/.env 或环境变量读取：
  BAIDU_AK / BAIDU_SK

常用可配置项：
  BAIDU_STAT_DOMAIN=gravatar.bluecdn.com
  BAIDU_STAT_START=2026-06-07
  BAIDU_STAT_METRIC=pv
  BAIDU_COUNT_FILE=/opt/gravatar-proxy/stats/baidu.count
"""
import datetime as dt
import hashlib
import hmac
import json
import os
import sys
import tempfile
import urllib.error
import urllib.parse
import urllib.request

HOST = "cdn.baidubce.com"


def load_env(path):
    try:
        with open(path, "r", encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if not line or line.startswith("#") or "=" not in line:
                    continue
                key, value = line.split("=", 1)
                key = key.strip()
                value = value.strip().strip("\"'")
                os.environ.setdefault(key, value)
    except FileNotFoundError:
        pass


def uri_encode(value, keep_slash=False):
    out = []
    for b in str(value).encode("utf-8"):
        c = chr(b)
        if c.isalnum() or c in "-_.~" or (c == "/" and keep_slash):
            out.append(c)
        else:
            out.append("%{:02X}".format(b))
    return "".join(out)


def canonical_query(params):
    if not params:
        return ""
    pairs = []
    for key, value in sorted(params.items()):
        if value is None:
            value = ""
        pairs.append(f"{uri_encode(key)}={uri_encode(value)}")
    return "&".join(pairs)


def auth(method, path, params, ak, sk):
    ts = dt.datetime.now(dt.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    prefix = f"bce-auth-v1/{ak}/{ts}/1800"
    signing_key = hmac.new(sk.encode(), prefix.encode(), hashlib.sha256).hexdigest()
    canon = "\n".join([
        method.upper(),
        uri_encode(path, keep_slash=True),
        canonical_query(params),
        f"host:{uri_encode(HOST)}",
    ])
    sig = hmac.new(signing_key.encode(), canon.encode(), hashlib.sha256).hexdigest()
    return f"{prefix}/host/{sig}"


def fetch_json(method, path, params, body, ak, sk):
    query = canonical_query(params)
    url = f"https://{HOST}{path}" + (f"?{query}" if query else "")
    data = None if body is None else json.dumps(body, separators=(",", ":")).encode("utf-8")
    req = urllib.request.Request(url, data=data, method=method.upper())
    req.add_header("Host", HOST)
    req.add_header("Authorization", auth(method, path, params, ak, sk))
    if data is not None:
        req.add_header("Content-Type", "application/json")
    with urllib.request.urlopen(req, timeout=30) as resp:
        raw = resp.read().decode("utf-8")
    return json.loads(raw) if raw else {}


def sum_metric(value, metric):
    if isinstance(value, dict):
        total = 0
        for key, child in value.items():
            if key == metric and isinstance(child, (int, float)):
                total += int(child)
            else:
                total += sum_metric(child, metric)
        return total
    if isinstance(value, list):
        return sum(sum_metric(item, metric) for item in value)
    return 0


def write_count(path, count):
    directory = os.path.dirname(path) or "."
    os.makedirs(directory, exist_ok=True)
    fd, tmp = tempfile.mkstemp(prefix=".baidu.count.", dir=directory)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            f.write(str(count))
        os.replace(tmp, path)
    finally:
        try:
            os.unlink(tmp)
        except FileNotFoundError:
            pass


def main():
    load_env(os.environ.get("BAIDU_ENV_FILE", "/opt/gravatar-proxy/.env"))

    ak = os.environ.get("BAIDU_AK")
    sk = os.environ.get("BAIDU_SK")
    if not ak or not sk:
        print("[baidu-stats] 缺少 BAIDU_AK 或 BAIDU_SK，跳过", file=sys.stderr)
        return 0

    domain = os.environ.get("BAIDU_STAT_DOMAIN", "gravatar.bluecdn.com")
    start = os.environ.get("BAIDU_STAT_START", "2026-06-07")
    end = os.environ.get("BAIDU_STAT_END") or dt.datetime.now(dt.timezone.utc).strftime("%Y-%m-%d")
    metric = os.environ.get("BAIDU_STAT_METRIC", "pv")
    out_file = os.environ.get("BAIDU_COUNT_FILE", "/opt/gravatar-proxy/stats/baidu.count")

    if len(start) == 10:
        start = start + "T00:00:00Z"
    if len(end) == 10:
        end = end + "T23:59:59Z"

    method = os.environ.get("BAIDU_STAT_METHOD", "GET")
    path = os.environ.get("BAIDU_STAT_PATH", f"/v2/stat/{metric}")
    params = json.loads(os.environ.get("BAIDU_STAT_PARAMS", json.dumps({
        "domain": domain,
        "startTime": start,
        "endTime": end,
        "period": "86400",
    })))
    body_env = os.environ.get("BAIDU_STAT_BODY")
    body = json.loads(body_env) if body_env else None

    try:
        resp = fetch_json(method, path, params, body, ak, sk)
    except urllib.error.HTTPError as e:
        print(f"[baidu-stats] 百度 CDN API 失败 {e.code}: {e.read().decode('utf-8', 'replace')}", file=sys.stderr)
        return 0
    except Exception as e:
        print(f"[baidu-stats] 百度 CDN API 调用失败: {e}", file=sys.stderr)
        return 0

    count = sum_metric(resp, metric)
    write_count(out_file, count)
    print(f"[baidu-stats] 已更新 {out_file} = {count}（{domain} {metric} {start}~{end}）")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
