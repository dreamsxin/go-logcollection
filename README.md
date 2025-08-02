# go-logcollection

## proto 生成

```shell
# 验证安装
protoc --version
# 安装protoc编译器
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
# 安装gRPC代码生成器
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
protoc --go_out=. --go_opt=paths=source_relative log.proto
protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative api/pb/log.proto
```
