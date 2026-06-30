BINARY  := gravatar-proxy
HOST    ?= debian@15.204.89.17
KEY     ?= ~/.ssh/gentpan.pem
REMOTE  := /opt/gravatar-proxy
# 部署用户(debian)对 REMOTE 无直接写权限，需 sudo；二进制先传到临时目录再 sudo 移动。
SSH     := ssh -i $(KEY) -o StrictHostKeyChecking=no
SCP     := scp -i $(KEY) -o StrictHostKeyChecking=no

.PHONY: build run linux deploy deploy-stats clean

# 本地构建（当前平台）
build:
	go build -ldflags="-s -w" -o $(BINARY) .

# 本地运行（监听 127.0.0.1:8787）
run:
	go run .

# 交叉编译 Linux amd64 部署二进制（go:embed 已把页面/资源打进二进制）
linux:
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/$(BINARY).linux-amd64 .

# 构建并部署到 OVH 源站（停服 → 上传 → 启服）
# binary 传到 /tmp 再 sudo 移动（/opt/gravatar-proxy 属 caddy，debian 无写权限）。
deploy: linux
	$(SCP) bin/$(BINARY).linux-amd64 $(HOST):/tmp/$(BINARY).new
	$(SSH) $(HOST) 'sudo install -o caddy -g caddy -m 0755 /tmp/$(BINARY).new $(REMOTE)/$(BINARY) && rm -f /tmp/$(BINARY).new && sudo systemctl restart $(BINARY) && sleep 1 && sudo systemctl status --no-pager $(BINARY)'

# 部署/更新 CDN 统计脚本与 systemd timer（不会写入密钥；密钥仍放远端 $(REMOTE)/.env）。
deploy-stats:
	$(SCP) deploy/bunny-stats.sh deploy/baidu-stats.py $(HOST):/tmp/
	$(SCP) deploy/gravatar-proxy.service deploy/bunny-stats.service deploy/bunny-stats.timer deploy/baidu-stats.service deploy/baidu-stats.timer $(HOST):/tmp/
	$(SSH) $(HOST) 'sudo install -o caddy -g caddy -m 0700 /tmp/bunny-stats.sh $(REMOTE)/bunny-stats.sh && sudo install -o caddy -g caddy -m 0700 /tmp/baidu-stats.py $(REMOTE)/baidu-stats.py && sudo install -o root -g root -m 0644 /tmp/gravatar-proxy.service /etc/systemd/system/gravatar-proxy.service && sudo install -o root -g root -m 0644 /tmp/bunny-stats.service /etc/systemd/system/bunny-stats.service && sudo install -o root -g root -m 0644 /tmp/bunny-stats.timer /etc/systemd/system/bunny-stats.timer && sudo install -o root -g root -m 0644 /tmp/baidu-stats.service /etc/systemd/system/baidu-stats.service && sudo install -o root -g root -m 0644 /tmp/baidu-stats.timer /etc/systemd/system/baidu-stats.timer && sudo systemctl daemon-reload && sudo systemctl enable --now bunny-stats.timer baidu-stats.timer && sudo systemctl restart $(BINARY) && sudo systemctl list-timers --no-pager "*stats.timer"'

clean:
	rm -f $(BINARY) bin/$(BINARY).linux-amd64
