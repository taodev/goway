
APP_NAME = goway

# 生成linux可执行文件
linux:
	GOOS=linux GOARCH=amd64 go build -o ./bin/$(APP_NAME)
	
# 生成windows可执行文件
windows:
	GOOS=windows GOARCH=amd64 go build -o ./bin/$(APP_NAME).exe

# 部署到服务器
deploy:
	cp -f ./bin/$(APP_NAME) /code/apps/ssh-tunnel/$(APP_NAME)

INSTALL_PATH = /code/apps/goway

install: 
	mkdir -p $(INSTALL_PATH)
	cp -f ./goway ./config.yaml ./id_goway ./id_goway.pub $(INSTALL_PATH)

# 默认生成linux可执行文件
default: linux
