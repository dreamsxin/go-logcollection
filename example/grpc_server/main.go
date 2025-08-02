package main

import (
	"fmt"
	"log"
	"log/slog"
	"time"

	"github.com/dreamsxin/go-logcollection"
)

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)
	writer, err := logcollection.NewLogWriter("./logs/app.log",
		logcollection.WithMaxSizeMB(20),             // 设置最大日志大小为20MB
		logcollection.WithBufferSize(2000),          // 设置通道缓冲区大小为2000
		logcollection.WithWriterBufferSize(64*1024), // 设置写入缓冲区为64KB
		logcollection.WithMaxAgeDays(7),             // 设置日志保留7天
	)

	if err != nil {
		log.Fatalf("Failed to create log writer: %v", err)
	}
	defer writer.Stop()

	// 启动gRPC服务器（在后台goroutine中）
	go func() {
		if err := logcollection.StartGRPCServer(":50051", writer); err != nil {
			log.Fatalf("Failed to start gRPC server: %v", err)
		}
	}()
	slog.Info("gRPC server started in background", "address", ":50051")

	// 本地日志生产示例
	startTime := time.Now().UnixMilli()
	for i := 0; i < 100000; i++ {
		entry := &logcollection.LogEntry{
			Timestamp: time.Now().Unix(),
			Level:     "INFO",
			Service:   "local-producer",
			Message:   fmt.Sprintf("Processing item %d", i),
			Tags: map[string]string{
				"source": "local",
			},
		}
		writer.Write(entry)
	}
	fmt.Printf("本地日志生产耗时: %dms\n", time.Now().UnixMilli()-startTime)

	// 保持程序运行以继续提供gRPC服务
	select {}
}
