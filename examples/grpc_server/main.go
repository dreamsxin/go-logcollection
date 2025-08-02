package main

import (
	"log"
	"log/slog"

	"github.com/dreamsxin/go-logcollection/internal/grpc"
	internallog "github.com/dreamsxin/go-logcollection/internal/log"
)

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)
	writer, err := internallog.NewLogWriter("./logs/app.log",
		internallog.WithMaxSizeMB(20),             // 设置最大日志大小为20MB
		internallog.WithBufferSize(2000),          // 设置通道缓冲区大小为2000
		internallog.WithWriterBufferSize(64*1024), // 设置写入缓冲区为64KB
		internallog.WithMaxAgeDays(7),             // 设置日志保留7天
	)

	if err != nil {
		log.Fatalf("Failed to create log writer: %v", err)
	}
	defer writer.Stop()

	// 启动gRPC服务器（在后台goroutine中）
	go func() {
		if err := grpc.StartGRPCServer(":50051", writer); err != nil {
			log.Fatalf("Failed to start gRPC server: %v", err)
		}
	}()
	slog.Info("gRPC server started in background", "address", ":50051")

	// 保持程序运行以继续提供gRPC服务
	select {}
}
