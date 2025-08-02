package main

import (
	"fmt"
	"log"
	"log/slog"
	"time"

	"github.com/dreamsxin/go-logcollection/api/pb"
	internallog "github.com/dreamsxin/go-logcollection/internal/log"
)

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)
	writer, err := internallog.NewLogWriter("./logs/app.log",
		internallog.WithMaxSizeMB(20),             // 设置最大日志大小为20MB
		internallog.WithBufferSize(2000),          // 设置通道缓冲区大小为2000
		internallog.WithWriterBufferSize(64*1024), // 设置写入缓冲区为64KB
	)

	if err != nil {
		log.Fatalf("Failed to create log writer: %v", err)
	}

	// 日志生产示例
	startTime := time.Now().UnixMilli()
	for i := 0; i < 100000; i++ {
		entry := &pb.LogEntry{
			Timestamp: time.Now().Unix(),
			Level:     "INFO",
			Message:   fmt.Sprintf("Processing item %d", i),
		}
		writer.Write(entry)
	}
	writer.Stop()
	fmt.Printf("耗时: %dms\n", time.Now().UnixMilli()-startTime)
}
