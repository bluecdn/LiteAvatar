BINARY  := gravatar-proxy
HOST    ?= root@43.173.85.48
KEY     ?= ~/.ssh/gentpan.pem
REMOTE  := /opt/gravatar-proxy

.PHONY: build run linux deploy clean

# 本地构建（当前平台）
build:
	go build -ldflags="-s -w" -o $(BINARY) .

# 本地运行（监听 127.0.0.1:8787）
run:
	go run .

# 交叉编译 Linux amd64 部署二进制
linux:
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/$(BINARY).linux-amd64 .

# 构建并部署到目标服务器（停服 → 上传 → 启服）
deploy: linux
	ssh -i $(KEY) $(HOST) 'systemctl stop $(BINARY)'
	scp -i $(KEY) bin/$(BINARY).linux-amd64 $(HOST):$(REMOTE)/$(BINARY)
	ssh -i $(KEY) $(HOST) 'systemctl start $(BINARY) && systemctl status --no-pager $(BINARY)'

clean:
	rm -f $(BINARY) bin/$(BINARY).linux-amd64
