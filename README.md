# LiteAvatar

多源头像聚合代理 —— Gravatar 国内加速。

按顺序探测 **Gravatar / Cravatar / WeAvatar / CNAvatar** 四个邮箱头像源，纯数字 ID 走**腾讯 QQ 头像**，自动返回第一个命中的真实头像；全部未命中时回退本地 Gravatar 官方蓝色默认头像。无 CORS 限制，适合 WordPress、论坛、博客等国内站点。

服务地址: `https://gravatar.bluecdn.com` · `https://gravatar.yite.net`

## 接口

```
GET /avatar/{id}?s={size}&d={default}
```

| 参数 | 说明 |
|------|------|
| `id` | **32–64 位十六进制** = 邮箱 hash(md5/sha256),串行探测四源;**5–12 位纯数字** = QQ 号,走 qlogo |
| `s` / `size` | 尺寸(像素),默认 `80`,上限 `2048`;所有来源统一输出精确尺寸 |
| `d`  | 未命中时透传给上游的默认图策略(gravatar 兼容: `404` / `mp` / `identicon` …) |

其它端点: `GET /stats` / `GET /stats.php`(累计请求数 JSON) · `GET /healthz`(健康检查)。展示值来自阿里云 ESA 原始日志，只累计合法头像 ID 的 `GET /avatar/{id}` 请求（包括边缘 HIT 与 MISS，不含首页、图标、扫描路径和 HEAD），并保留源站本地计数用于排查。

响应头 `X-Avatar-Source` 标记实际命中的源(`cravatar` / `gravatar` / `weavatar` / `qq` / `default`)。

### 示例

```html
<!-- 邮箱头像(hash = sha256 of email) -->
<img src="https://gravatar.bluecdn.com/avatar/5b2c80ac69782ece5fc8829a16ec90a31d47de643d036066458d6ea1db4e5684?s=160">
<!-- QQ 头像 -->
<img src="https://gravatar.bluecdn.com/avatar/10001?s=100">
```

## 工作原理

```
                 ┌─ 并发探测 ?s=1&d=404 ─┐
/avatar/{hash} ──┤  cn.cravatar.com      │
   (邮箱 hash)    │  secure.gravatar.com  ├─→ 取优先级最高的命中源 ─→ 回源拉取真实头像
                 │  weavatar.com         │                          (X-Avatar-Source)
                 │  cnavatar.com         │
                 └───────────────────────┘
/avatar/{qq}  ── q.qlogo.cn/headimg_dl (spec 按尺寸选择)
全部未命中     ── 内置 Gravatar 官方蓝色 default-avatar.avif
```

探测使用 `d=404` 判断各源是否真有该头像；CNAvatar 会忽略该参数，因此服务会以同尺寸哨兵图哈希排除其占位图。QQ 的离散尺寸档位会在本地缩放到请求的精确尺寸。结果由前置 CDN 边缘缓存 7 天；浏览器收到 `max-age=0, must-revalidate`，重新加载页面会再次请求 CDN 并进入统计，同时 `s-maxage` 允许 CDN 命中而不回源。

## 构建 / 运行

```bash
make build && ./gravatar-proxy        # 本地，监听 127.0.0.1:8787
make run                              # go run ./app/backend
make linux                            # 交叉编译 Linux amd64 → bin/
make deploy HOST=root@65.109.62.100   # 编译并部署到当前源站
```

参数: `-listen`(默认 `127.0.0.1:8787`) · `-site-dir`(默认当前目录，读取根目录 `index.html` 与 `public/`) · `-counter`(计数持久化文件)。

## 项目结构

```
index.html          # 首页
public/             # favicon、manifest 等前端静态资源
app/backend/        # Go 头像代理后端
stats/              # CDN 统计脚本与运行时 *.count 计数文件
deploy/             # systemd / Caddy 部署配置
```

## 部署架构

后端必须部署在能直连各头像上游的**境外节点**(国内无法访问 gravatar.com)。前置 Caddy 终止 TLS、限制 `/avatar/{hash}` 路径并反代到本服务,再由 CDN 做边缘缓存与分发。参见 [`deploy/`](deploy/):

- `deploy/Caddyfile.reference` — Caddy 站点配置(路径白名单、CORS、安全头)
- `deploy/gravatar-proxy.service` — systemd 单元

> 历史: 本服务原部署于腾讯云新加坡节点，当前运行于 `65.109.62.100`。
