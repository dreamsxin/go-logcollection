package logcollection

import (
	"fmt"
	"testing"
	"time"
)

func TestLogCollectionWriter(t *testing.T) {
	// 初始化组件
	writer := NewLogWriter("./logs/app.log", 100, 1000) // 100MB滚动

	// 日志生产示例
	for i := 0; i < 100; i++ {
		entry := &LogEntry{
			Timestamp: time.Now().Unix(),
			Level:     "INFO",
			Message:   fmt.Sprintf("Processing item %d", i),
		}
		writer.Write(entry)
	}
	writer.Stop()
}

func TestLogCollectionReader(t *testing.T) {
	// 日志读取
	reader := NewLogReader("./logs")

	// 定义日志处理函数
	processLog := func(log *LogEntry) {
		fmt.Printf("处理日志: %#v\n", log.String())
	}

	// 启动日志跟踪
	reader.Read(100, processLog)
}
