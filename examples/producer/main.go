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
	)

	if err != nil {
		log.Fatalf("Failed to create log writer: %v", err)
	}

	// 日志生产示例
	startTime := time.Now().UnixMilli()
	for i := 0; i < 100000; i++ {
		entry := &logcollection.LogEntry{
			Timestamp: time.Now().Unix(),
			Level:     "INFO",
			Message:   fmt.Sprintf("Processing item %d", i),
		}
		writer.Write(entry)
	}
	writer.Stop()
	fmt.Printf("耗时: %dms\n", time.Now().UnixMilli()-startTime)
}
