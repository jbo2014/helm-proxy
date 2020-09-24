# 预备条件
服务器需要已经安装helm、helm-push

# helm-proxy
Rest api for helm 3
参考[opskumu/helm-wrapper](https://github.com/opskumu/helm-wrapper)，并在此基础上添加其他api接口

# 编译
windows：
go build -ldflags "-s -w" -o xxxx

linux:
GOOS=linux 
GOARCH=amd64 
go build -ldflags "-s -w" -o xxxx

docker：
GOOS=linux 
GOARCH=amd64 
go build -ldflags "-s -w" -o xxxx
docker build -t helm-proxy:`git rev-parse --short HEAD` .

# 运行
./helm-proxy --config </path/to/config.yaml> --kubeconfig </path/to/kubeconfig>  
例子：后台执行，自定义ip、port  
nohup ./helm-proxy --config ./config.yaml --kubeconfig /root/.kube/config --addr 192.168.0.188 --port 18080 &  