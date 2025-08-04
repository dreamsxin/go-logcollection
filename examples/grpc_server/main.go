package main

import (
	"log"
	"log/slog"

	"github.com/dreamsxin/go-logcollection/pkg/grpc"
	pkglog "github.com/dreamsxin/go-logcollection/pkg/log"
)

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)
	writer, err := pkglog.NewLogWriter("./logs/app.log",
		pkglog.WithMaxSizeMB(20),             // 设置最大日志大小为20MB
		pkglog.WithBufferSize(2000),          // 设置通道缓冲区大小为2000
		pkglog.WithWriterBufferSize(64*1024), // 设置写入缓冲区为64KB
		pkglog.WithMaxAgeDays(7),             // 设置日志保留7天
	)

	if err != nil {
		log.Fatalf("Failed to create log writer: %v", err)
	}
	defer writer.Stop()

	// 启动gRPC服务器（在后台goroutine中）
	slog.Info("gRPC server started in background", "address", ":50051")
	if err := grpc.StartGRPCServer(":50051", writer); err != nil {
		log.Fatalf("Failed to start gRPC server: %v", err)
	}

}
