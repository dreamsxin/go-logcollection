package main

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/dreamsxin/go-logcollection"
)

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)
	reader := logcollection.NewLogReader("../producer/logs")

	// 日志生产示例
	startTime := time.Now().UnixMilli()

	// 定义日志处理函数
	processLog := func(log *logcollection.LogEntry) {
		fmt.Printf("处理日志: %#v\n", log.String())
	}

	// 启动日志跟踪
	reader.ReadAll(processLog)
	fmt.Printf("耗时: %dms\n", time.Now().UnixMilli()-startTime)
}
