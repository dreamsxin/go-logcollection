package grpc

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/dreamsxin/go-logcollection/api/pb"
	"github.com/dreamsxin/go-logcollection/internal/log"
	"google.golang.org/grpc"
)

// LogServerOptions 定义LogServer的配置选项
type LogServerOptions struct {
	WriteTimeout time.Duration // 写入超时时间
	RetryCount   int           // 重试次数
}

// WithWriteTimeout 设置写入超时时间
func WithWriteTimeout(timeout time.Duration) func(*LogServerOptions) {
	return func(o *LogServerOptions) {
		o.WriteTimeout = timeout
	}
}

// WithRetryCount 设置重试次数
func WithRetryCount(count int) func(*LogServerOptions) {
	return func(o *LogServerOptions) {
		o.RetryCount = count
	}
}

// LogServer 实现LogServiceServer接口
type LogServer struct {
	pb.UnimplementedLogServiceServer
	logWriter *log.LogWriter
	options   LogServerOptions
	mu        sync.Mutex
}

// NewLogServer 创建新的日志gRPC服务器
func NewLogServer(logWriter *log.LogWriter, opts ...func(*LogServerOptions)) (*LogServer, error) {
	// 参数验证
	if logWriter == nil {
		return nil, fmt.Errorf("logWriter cannot be nil")
	}

	// 默认选项
	options := LogServerOptions{
		WriteTimeout: 5 * time.Second,
		RetryCount:   3,
	}

	// 应用选项
	for _, opt := range opts {
		opt(&options)
	}

	return &LogServer{
		UnimplementedLogServiceServer: pb.UnimplementedLogServiceServer{},
		logWriter:                     logWriter,
		options:                       options,
	}, nil
}

// SubmitLog 处理单个日志条目提交
func (s *LogServer) SubmitLog(ctx context.Context, entry *pb.LogEntry) (*pb.SubmitLogResponse, error) {
	// 添加超时控制
	ctx, cancel := context.WithTimeout(ctx, s.options.WriteTimeout)
	defer cancel()

	var err error
	for i := 0; i < s.options.RetryCount; i++ {
		err = s.logWriter.Write(entry)
		if err == nil {
			return &pb.SubmitLogResponse{
				Success:   true,
				Message:   "Log entry written successfully",
				Timestamp: time.Now().Unix(),
			}, nil
		}
		// 指数退避重试
		time.Sleep(time.Duration(1<<i) * 100 * time.Millisecond)
	}

	slog.Error("Failed to write log entry after retries", "error", err)
	return &pb.SubmitLogResponse{
		Success:   false,
		Message:   fmt.Sprintf("Failed to write log: %v", err),
		Timestamp: time.Now().Unix(),
	}, err
}

// SubmitLogs 处理流式日志条目提交
func (s *LogServer) SubmitLogs(stream pb.LogService_SubmitLogsServer) error {
	count := 0

	for {
		entry, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				// 客户端已发送所有日志
				return stream.SendAndClose(&pb.SubmitLogsResponse{
					Success:   true,
					Message:   "All log entries processed",
					Count:     int64(count),
					Timestamp: time.Now().Unix(),
				})
			}
			slog.Error("Failed to receive log entry", "error", err)
			return err
		}

		// 使用LogWriter的写入方法，利用其内部批量处理
		var writeErr error
		for i := 0; i < s.options.RetryCount; i++ {
			writeErr = s.logWriter.Write(entry)
			if writeErr == nil {
				break
			}
			time.Sleep(time.Duration(1<<i) * 100 * time.Millisecond)
		}

		if writeErr != nil {
			slog.Error("Failed to write log entry after retries", "error", writeErr)
			return writeErr
		}

		count++
	}
}

// StartGRPCServer 启动gRPC服务器
func StartGRPCServer(addr string, logWriter *log.LogWriter, opts ...func(*LogServerOptions)) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("Failed to listen", "error", err)
		return err
	}

	server, err := NewLogServer(logWriter, opts...)
	if err != nil {
		slog.Error("Failed to create log server", "error", err)
		return err
	}

	s := grpc.NewServer()
	pb.RegisterLogServiceServer(s, server)

	slog.Info("Starting gRPC server", "address", addr)
	if err := s.Serve(lis); err != nil {
		slog.Error("Failed to serve", "error", err)
		return err
	}
	return nil
}
