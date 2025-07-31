# go-logcollection

## proto 生成

```shell
# 验证安装
protoc --version
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
protoc --go_out=. --go_opt=paths=source_relative log.proto
```