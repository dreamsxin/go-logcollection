package logcollection

import (
	"fmt"
	"testing"
	"time"

	"github.com/dreamsxin/go-logcollection/api/pb"
	pkglog "github.com/dreamsxin/go-logcollection/pkg/log"
)

func TestLogCollectionWriter(t *testing.T) {
	// 初始化组件
	writer, err := pkglog.NewLogWriter("./logs/app.log",
		pkglog.WithMaxSizeMB(20),             // 设置最大日志大小为20MB
		pkglog.WithBufferSize(2000),          // 设置通道缓冲区大小为2000
		pkglog.WithWriterBufferSize(64*1024), // 设置写入缓冲区为64KB
	)

	if err != nil {
		t.Fatal(err)
	}

	// 日志生产示例
	for i := 0; i < 100; i++ {
		entry := &pb.LogEntry{
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
	reader, err := pkglog.NewLogReader("./logs")
	if err != nil {
		t.Fatal(err)
	}
	// 定义日志处理函数
	processLog := func(log *pb.LogEntry) {
		fmt.Printf("处理日志: %#v\n", log.String())
	}

	// 启动日志跟踪
	reader.Read(100, processLog)
}

// 写入性能测试
func BenchmarkLogCollectionWriter(b *testing.B) {
	writer, err := pkglog.NewLogWriter("./logs/app.log",
		pkglog.WithMaxSizeMB(100),            // 设置最大日志大小为20MB
		pkglog.WithBufferSize(2000),          // 设置通道缓冲区大小为2000
		pkglog.WithWriterBufferSize(64*1024), // 设置写入缓冲区为64KB
	)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer() // 重置计时器
	for i := 0; i < b.N; i++ {
		entry := &pb.LogEntry{
			Timestamp: time.Now().Unix(),
			Level:     "INFO",
			Message:   fmt.Sprintf("Processing item %d", i),
		}
		writer.Write(entry)
	}
	writer.Stop()
}

// 读取性能测试
func BenchmarkLogCollectionReader(b *testing.B) {
	reader, err := pkglog.NewLogReader("./logs")
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer() // 重置计时器
	reader.Read(int64(b.N), func(log *pb.LogEntry) {
		// 处理日志
	})
}
