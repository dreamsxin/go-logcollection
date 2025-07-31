package main

import (
	"fmt"
	"time"

	"github.com/dreamsxin/go-logcollection"
)

func main() {
	writer := logcollection.NewLogWriter("./logs/app.log", 100, 1000) // 100MB滚动

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
