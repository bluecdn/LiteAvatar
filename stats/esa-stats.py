#!/usr/bin/env python3
"""Count valid LiteAvatar GET requests from Alibaba Cloud ESA raw logs.

The output is cumulative across processed log files. ESA edge HIT and MISS
requests are both present in the logs; non-avatar paths, invalid IDs and HEAD
checks are excluded.
"""

import concurrent.futures
import datetime as dt
import gzip
import json
import os
import pathlib
import re
import tempfile
import urllib.parse
import urllib.request


UTC = dt.timezone.utc
SCHEMA_VERSION = 2
METRIC = "valid_avatar_get_requests"
AVATAR_PATH_RE = re.compile(
    r"^/avatar/(?:[0-9]{5,12}|[a-f0-9]{32,64})(?:\.(?:jpg|png|avif))?$",
    re.IGNORECASE,
)


def iso(value: dt.datetime) -> str:
    return value.astimezone(UTC).strftime("%Y-%m-%dT%H:%M:%SZ")


def atomic_json(path: pathlib.Path, value: object, mode: int) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, temp_path = tempfile.mkstemp(prefix=path.name + ".", dir=path.parent)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as handle:
            json.dump(value, handle, ensure_ascii=True, separators=(",", ":"))
            handle.write("\n")
        os.chmod(temp_path, mode)
        os.replace(temp_path, path)
    finally:
        if os.path.exists(temp_path):
            os.unlink(temp_path)


def request_path(record: dict) -> str:
    uri = str(record.get("ClientRequestURI") or record.get("ClientRequestPath") or "")
    return urllib.parse.unquote(urllib.parse.urlsplit(uri).path)


def is_valid_avatar_request(record: dict, hostname: str) -> bool:
    return (
        str(record.get("ClientRequestHost", "")).lower() == hostname
        and str(record.get("ClientRequestMethod", "GET")).upper() == "GET"
        and AVATAR_PATH_RE.fullmatch(request_path(record)) is not None
    )


def download_count(item: tuple[str, str], hostname: str) -> tuple[str, int]:
    name, url = item
    url = url.strip().replace(" ", "")
    if not url.startswith("http"):
        url = "https://" + url
    count = 0
    with urllib.request.urlopen(url, timeout=60) as response:
        with gzip.GzipFile(fileobj=response) as archive:
            for raw_line in archive:
                try:
                    record = json.loads(raw_line)
                except (json.JSONDecodeError, UnicodeDecodeError):
                    continue
                if is_valid_avatar_request(record, hostname):
                    count += 1
    return name, count


def parse_start(value: str, fallback: dt.datetime) -> dt.datetime:
    if not value:
        return fallback
    try:
        parsed = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        parsed = dt.datetime.strptime(value, "%Y-%m-%d")
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=UTC)
    return parsed.astimezone(UTC)


def main() -> int:
    # Import the vendor SDK only in the executable path, so matcher tests do not
    # require cloud dependencies on a developer workstation.
    from alibabacloud_esa20240910.client import Client
    from alibabacloud_esa20240910 import models
    from alibabacloud_tea_openapi import models as open_api_models

    access_key_id = os.environ["ALIYUN_ACCESS_KEY_ID"]
    access_key_secret = os.environ["ALIYUN_ACCESS_KEY_SECRET"]
    region = os.environ.get("DEFAULT_REGION", "cn-hangzhou")
    site_name = os.environ.get("ESA_SITE_NAME", "bluecdn.com")
    hostname = os.environ.get("ESA_STAT_HOST", "gravatar.bluecdn.com").lower()
    lookback_days = min(int(os.environ.get("ESA_LOG_LOOKBACK_DAYS", "30")), 30)
    workers = max(1, min(int(os.environ.get("ESA_LOG_WORKERS", "12")), 24))
    output = pathlib.Path(os.environ.get(
        "ESA_COUNT_FILE", "/opt/gravatar-proxy/stats/esa.count"))
    state_path = pathlib.Path(os.environ.get(
        "ESA_STATE_FILE", "/opt/gravatar-proxy/stats/esa-avatar-state.json"))

    config = open_api_models.Config(
        access_key_id=access_key_id,
        access_key_secret=access_key_secret,
        region_id=region,
    )
    client = Client(config)
    sites = client.list_sites(models.ListSitesRequest(
        site_name=site_name,
        page_number=1,
        page_size=100,
    )).body.sites or []
    site = next((item for item in sites if item.site_name == site_name), None)
    if site is None:
        raise RuntimeError(f"ESA site not found: {site_name}")

    now = dt.datetime.now(UTC).replace(microsecond=0)
    retained_start = now - dt.timedelta(days=lookback_days)
    configured_start = parse_start(os.environ.get("ESA_STAT_START", ""), retained_start)
    start = max(retained_start, configured_start)
    logs: dict[str, str] = {}
    page = 1
    while True:
        response = client.describe_site_logs(models.DescribeSiteLogsRequest(
            site_id=site.site_id,
            start_time=iso(start),
            end_time=iso(now),
            page_number=page,
            page_size=1000,
        )).body
        total_count = 0
        for detail in response.site_log_details or []:
            total_count = max(total_count, detail.page_infos.total_count or 0)
            for info in detail.log_infos or []:
                if info.log_name and info.log_path:
                    logs[info.log_name] = info.log_path
        if page * 1000 >= total_count:
            break
        page += 1

    state = {
        "schema_version": SCHEMA_VERSION,
        "metric": METRIC,
        "hostname": hostname,
        "total": 0,
        "processed": [],
    }
    if state_path.exists():
        with state_path.open("r", encoding="utf-8") as handle:
            saved = json.load(handle)
        if (
            saved.get("schema_version") == SCHEMA_VERSION
            and saved.get("metric") == METRIC
            and saved.get("hostname") == hostname
        ):
            state = saved
    processed = set(state.get("processed", []))
    pending = [(name, url) for name, url in logs.items() if name not in processed]

    added = 0
    failures = 0
    with concurrent.futures.ThreadPoolExecutor(max_workers=workers) as pool:
        futures = [pool.submit(download_count, item, hostname) for item in pending]
        for future in concurrent.futures.as_completed(futures):
            try:
                name, count = future.result()
            except Exception as exc:
                failures += 1
                print(f"[esa-stats] log download failed: {exc}", flush=True)
                continue
            processed.add(name)
            added += count

    total = int(state.get("total", 0)) + added
    atomic_json(state_path, {
        "schema_version": SCHEMA_VERSION,
        "metric": METRIC,
        "hostname": hostname,
        "total": total,
        "processed": sorted(processed),
        "updated_at": iso(now),
    }, 0o600)
    if failures == 0:
        atomic_json(output, total, 0o644)
    print(
        f"[esa-stats] total={total} added={added} files={len(pending)} "
        f"failed={failures} metric={METRIC} host={hostname} site={site.site_id}",
        flush=True,
    )
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
