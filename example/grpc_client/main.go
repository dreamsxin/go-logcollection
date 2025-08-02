package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"time"

	"github.com/dreamsxin/go-logcollection"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)

	// 连接gRPC服务器
	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := logcollection.NewLogServiceClient(conn)

	// 单个日志提交示例
	singleLogExample(client)

	// 流式日志提交示例
	streamLogExample(client)
}

func singleLogExample(client logcollection.LogServiceClient) {
	entry := &logcollection.LogEntry{
		Timestamp: time.Now().Unix(),
		Level:     "INFO",
		Service:   "grpc-client",
		Message:   "Single log entry from gRPC client",
		Tags: map[string]string{
			"source": "grpc",
			"type":   "single",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	response, err := client.SubmitLog(ctx, entry)
	if err != nil {
		log.Fatalf("Failed to submit log: %v", err)
	}

	log.Printf("Single log response: %+v", response)
}

func streamLogExample(client logcollection.LogServiceClient) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.SubmitLogs(ctx)
	if err != nil {
		log.Fatalf("Failed to create stream: %v", err)
	}

	// 发送多个日志条目
	for i := 0; i < 5; i++ {
		entry := &logcollection.LogEntry{
			Timestamp: time.Now().Unix(),
			Level:     "DEBUG",
			Service:   "grpc-client",
			Message:   fmt.Sprintf("Stream log entry %d from gRPC client", i),
			Tags: map[string]string{
				"source": "grpc",
				"type":   "stream",
				"batch":  fmt.Sprintf("%d", i/10),
			},
		}

		if err := stream.Send(entry); err != nil {
			log.Fatalf("Failed to send log: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	response, err := stream.CloseAndRecv()
	if err != nil {
		log.Fatalf("Failed to receive stream response: %v", err)
	}

	log.Printf("Stream log response: %+v", response)
}
