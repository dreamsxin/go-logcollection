package logcollection

import (
	"context"
	"io"
	"log/slog"
	"net"
	"time"

	"google.golang.org/grpc"
)

// LogServer 实现LogServiceServer接口
type LogServer struct {
	UnimplementedLogServiceServer
	logWriter *LogWriter
}

// NewLogServer 创建新的日志gRPC服务器
func NewLogServer(logWriter *LogWriter) *LogServer {
	return &LogServer{
		logWriter: logWriter,
	}
}

// SubmitLog 处理单个日志条目提交
func (s *LogServer) SubmitLog(ctx context.Context, entry *LogEntry) (*SubmitLogResponse, error) {
	if err := s.logWriter.Write(entry); err != nil {
		slog.Error("Failed to write log entry", "error", err)
		return &SubmitLogResponse{
			Success:   false,
			Message:   err.Error(),
			Timestamp: time.Now().Unix(),
		}, err
	}

	return &SubmitLogResponse{
		Success:   true,
		Message:   "Log entry written successfully",
		Timestamp: time.Now().Unix(),
	}, nil
}

// SubmitLogs 处理流式日志条目提交
func (s *LogServer) SubmitLogs(stream LogService_SubmitLogsServer) error {
	count := 0

	for {
		entry, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				// 客户端已发送所有日志
				return stream.SendAndClose(&SubmitLogsResponse{
					Success:   true,
					Message:   "All log entries processed",
					Count:     int64(count),
					Timestamp: time.Now().Unix(),
				})
			}
			slog.Error("Failed to receive log entry", "error", err)
			return err
		}

		if err := s.logWriter.Write(entry); err != nil {
			slog.Error("Failed to write log entry", "error", err)
			return err
		}
		count++
	}
}

// StartGRPCServer 启动gRPC服务器
func StartGRPCServer(addr string, logWriter *LogWriter) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("Failed to listen", "error", err)
		return err
	}

	s := grpc.NewServer()
	RegisterLogServiceServer(s, NewLogServer(logWriter))

	slog.Info("Starting gRPC server", "address", addr)
	if err := s.Serve(lis); err != nil {
		slog.Error("Failed to serve", "error", err)
		return err
	}
	return nil
}
