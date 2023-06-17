
APP_NAME=goway
DIR=$(dirname "$0")

GOPATH=$(shell go env GOBIN)

# 生成linux可执行文件
linux:
	GOOS=linux GOARCH=amd64 go build -o ./bin/$(APP_NAME)
	
# 生成windows可执行文件
windows:
	GOOS=windows GOARCH=amd64 go build -o ./bin/$(APP_NAME).exe

# 部署到服务器
deploy:
	cp -f ./bin/$(APP_NAME) /code/apps/ssh-tunnel/$(APP_NAME)

sshkey:
	ssh-keygen -t ed25519 -f ./release/id_goway -C "goway"

install: 
	go install -v -trimpath -ldflags "-s -w -buildid="

	sudo cp $(GOPATH)/goway /usr/local/bin
	# sudo mkdir -p /usr/local/etc/goway
	# sudo cp ./release/config.yaml /usr/local/etc/goway
	# sudo cp ./release/id_goway /usr/local/etc/goway
	# sudo cp ./release/id_goway.pub /usr/local/etc/goway
	# sudo cp ./release/goway.service /etc/systemd/system
	sudo systemctl daemon-reload

# 默认生成linux可执行文件
default: linux
