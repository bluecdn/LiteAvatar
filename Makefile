BINARY  := gravatar-proxy
HOST    ?= root@65.109.62.100
KEY     ?= ~/.ssh/gentpan.pem
REMOTE  := /opt/gravatar-proxy
# 二进制先传到临时目录，再原子安装到 REMOTE。
SSH     := ssh -i $(KEY) -o StrictHostKeyChecking=no
SCP     := scp -i $(KEY) -o StrictHostKeyChecking=no

.PHONY: build run linux deploy deploy-stats deploy-esa-stats clean

# 本地构建（当前平台）
build:
	go build -ldflags="-s -w" -o $(BINARY) ./app/backend

# 本地运行（监听 127.0.0.1:8787）
run:
	go run ./app/backend

# 交叉编译 Linux amd64 部署二进制
linux:
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/$(BINARY).linux-amd64 ./app/backend

# 构建并部署到 Hetzner 源站（上传 → 原子替换 → 重启）。
deploy: linux
	$(SCP) bin/$(BINARY).linux-amd64 $(HOST):/tmp/$(BINARY).new
	$(SCP) index.html $(HOST):/tmp/liteavatar-index.html
	$(SCP) -r public $(HOST):/tmp/liteavatar-public
	$(SSH) $(HOST) 'sudo install -o caddy -g caddy -m 0755 /tmp/$(BINARY).new $(REMOTE)/$(BINARY) && sudo install -o caddy -g caddy -m 0644 /tmp/liteavatar-index.html $(REMOTE)/index.html && sudo rm -rf $(REMOTE)/public && sudo install -o caddy -g caddy -d $(REMOTE)/public $(REMOTE)/stats && sudo cp -a /tmp/liteavatar-public/. $(REMOTE)/public/ && sudo chown -R caddy:caddy $(REMOTE)/public && rm -rf /tmp/$(BINARY).new /tmp/liteavatar-index.html /tmp/liteavatar-public && sudo systemctl restart $(BINARY) && sleep 1 && sudo systemctl status --no-pager $(BINARY)'

# 部署/更新 CDN 统计脚本与 systemd timer（不会写入密钥；密钥仍放远端 $(REMOTE)/.env）。
deploy-stats:
	$(SCP) stats/bunny-stats.sh stats/baidu-stats.py $(HOST):/tmp/
	$(SCP) deploy/gravatar-proxy.service deploy/bunny-stats.service deploy/bunny-stats.timer deploy/baidu-stats.service deploy/baidu-stats.timer $(HOST):/tmp/
	$(SSH) $(HOST) 'sudo install -o caddy -g caddy -d $(REMOTE)/stats && sudo install -o caddy -g caddy -m 0700 /tmp/bunny-stats.sh $(REMOTE)/stats/bunny-stats.sh && sudo install -o caddy -g caddy -m 0700 /tmp/baidu-stats.py $(REMOTE)/stats/baidu-stats.py && sudo install -o root -g root -m 0644 /tmp/gravatar-proxy.service /etc/systemd/system/gravatar-proxy.service && sudo install -o root -g root -m 0644 /tmp/bunny-stats.service /etc/systemd/system/bunny-stats.service && sudo install -o root -g root -m 0644 /tmp/bunny-stats.timer /etc/systemd/system/bunny-stats.timer && sudo install -o root -g root -m 0644 /tmp/baidu-stats.service /etc/systemd/system/baidu-stats.service && sudo install -o root -g root -m 0644 /tmp/baidu-stats.timer /etc/systemd/system/baidu-stats.timer && sudo systemctl daemon-reload && sudo systemctl enable --now bunny-stats.timer baidu-stats.timer && sudo systemctl restart $(BINARY) && sudo systemctl list-timers --no-pager "*stats.timer"'

# 部署 ESA 有效头像请求统计；首次执行会从 ESA 当前可用的原始日志重建正确口径。
deploy-esa-stats:
	$(SCP) stats/esa-stats.py deploy/esa-stats.service deploy/esa-stats.timer $(HOST):/tmp/
	$(SSH) $(HOST) 'sudo install -o caddy -g caddy -d $(REMOTE)/stats && sudo install -o caddy -g caddy -m 0700 /tmp/esa-stats.py $(REMOTE)/stats/esa-stats.py && sudo install -o root -g root -m 0644 /tmp/esa-stats.service /etc/systemd/system/esa-stats.service && sudo install -o root -g root -m 0644 /tmp/esa-stats.timer /etc/systemd/system/esa-stats.timer && sudo systemctl daemon-reload && sudo systemctl enable --now esa-stats.timer && sudo systemctl start esa-stats.service && sudo systemctl status --no-pager esa-stats.service'

clean:
	rm -f $(BINARY) bin/$(BINARY).linux-amd64
