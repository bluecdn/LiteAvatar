BINARY  := gravatar-proxy
HOST    ?= debian@15.204.89.17
KEY     ?= ~/.ssh/gentpan.pem
REMOTE  := /opt/gravatar-proxy
# 部署用户(debian)对 REMOTE 无直接写权限，需 sudo；二进制先传到临时目录再 sudo 移动。
SSH     := ssh -i $(KEY) -o StrictHostKeyChecking=no
SCP     := scp -i $(KEY) -o StrictHostKeyChecking=no

.PHONY: build run linux deploy clean

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

clean:
	rm -f $(BINARY) bin/$(BINARY).linux-amd64
