package logcollection

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
)

// LogWriterOptions 定义LogWriter的配置选项
type LogWriterOptions struct {
	MaxSizeMB        int // 日志文件最大大小(MB)
	BufferSize       int // 通道缓冲区大小
	WriterBufferSize int // 写入器缓冲区大小(字节)
}

// WithMaxSizeMB 设置日志文件最大大小选项
func WithMaxSizeMB(maxSizeMB int) func(*LogWriterOptions) {
	return func(o *LogWriterOptions) {
		o.MaxSizeMB = maxSizeMB
	}
}

// WithBufferSize 设置通道缓冲区大小选项
func WithBufferSize(bufferSize int) func(*LogWriterOptions) {
	return func(o *LogWriterOptions) {
		o.BufferSize = bufferSize
	}
}

// WithWriterBufferSize 设置写入器缓冲区大小选项
func WithWriterBufferSize(bufferSize int) func(*LogWriterOptions) {
	return func(o *LogWriterOptions) {
		o.WriterBufferSize = bufferSize
	}
}

type LogWriter struct {
	path        string
	file        *os.File
	writer      *bufio.Writer // 新增缓冲写入器
	currentSize int64
	maxSize     int64
	mu          sync.Mutex
	buffer      chan proto.Message
	stopChan    chan struct{}
	wg          sync.WaitGroup
	closed      bool
}

func NewLogWriter(path string, opts ...func(*LogWriterOptions)) (*LogWriter, error) {
	// 设置默认选项
	options := LogWriterOptions{
		MaxSizeMB:        10,        // 默认10MB
		BufferSize:       1000,      // 默认通道缓冲区大小
		WriterBufferSize: 32 * 1024, // 默认32KB写入缓冲区
	}

	// 应用用户提供的选项
	for _, opt := range opts {
		opt(&options)
	}

	dir := filepath.Dir(path)
	_, err := os.Stat(dir)
	if err != nil { // 创建目录时检查错误
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			slog.Error("Failed to create log directory", "error", err)
			return nil, err
		}
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		slog.Error("Failed to open log file", "error", err)
		return nil, err // 或适当处理错误
	}

	// 获取文件信息以初始化currentSize
	fileInfo, err := file.Stat()
	if err != nil {
		slog.Error("Failed to get file info", "error", err)
		file.Close()
		return nil, err
	}

	w := &LogWriter{
		path:        path,
		file:        file,
		writer:      bufio.NewWriterSize(file, options.WriterBufferSize), // 32KB缓冲区
		currentSize: fileInfo.Size(),                                     // 初始化当前文件大小
		maxSize:     int64(options.MaxSizeMB) * 1024 * 1024,
		buffer:      make(chan proto.Message, options.BufferSize),
		stopChan:    make(chan struct{}),
	}

	w.wg.Add(1)
	go w.batchWriter()
	return w, nil
}

func (w *LogWriter) Write(entry proto.Message) error {
	if w.closed {
		return fmt.Errorf("writer is closed")
	}
	select {
	case w.buffer <- entry:
		return nil
	default:
		return fmt.Errorf("buffer is full") // 非阻塞写入，返回错误
	}
}

func (w *LogWriter) Stop() {
	w.closed = true
	close(w.stopChan)
	close(w.buffer)
	w.wg.Wait()
}

// Pack 将protobuf消息打包为length-prefix格式
func Pack(msg proto.Message) ([]byte, error) {
	data, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.BigEndian, uint32(len(data))); err != nil {
		return nil, err
	}
	if _, err := buf.Write(data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Unpack 从length-prefix格式数据流解析protobuf消息
func Unpack(r io.Reader, msg proto.Message) error {
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return err
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return err
	}
	return proto.Unmarshal(data, msg)
}

func (w *LogWriter) batchWriter() {
	defer w.wg.Done()

	batch := make([]byte, 0, 16*1024)
	ticker := time.NewTicker(500 * time.Millisecond) // 延长ticker间隔

	for {
		select {
		case entry, ok := <-w.buffer:
			if !ok {
				w.flush(batch)
				w.file.Close()
				return
			}
			data, err := Pack(entry)
			if err != nil {
				slog.Error("Failed to pack log entry")
				continue
			}
			batch = append(batch, data...)
			if len(batch) > 16*1024 {
				w.flush(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				w.flush(batch)
				batch = batch[:0] // 重置切片而非创建新切片
			}
		case <-w.stopChan:
			// 处理剩余数据
			for entry := range w.buffer {
				data, err := Pack(entry)
				if err != nil {
					slog.Error("Failed to pack log entry", "error", err)
					continue
				}
				batch = append(batch, data...)
			}
			w.flush(batch)
			w.file.Close()
			return
		}
	}
}

func (w *LogWriter) flush(data []byte) {
	if len(data) == 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock() // 确保整个写入过程都持有锁

	// 仅在必要时持有锁
	w.currentSize += int64(len(data))
	needRotate := w.currentSize >= w.maxSize

	// 在锁保护下执行写入
	if _, err := w.writer.Write(data); err != nil {
		slog.Error("Failed to write log data", "error", err)
		return
	}
	if err := w.writer.Flush(); err != nil {
		slog.Error("Failed to flush log data", "error", err)
		return
	}

	if needRotate {
		w.rotateFile()
	}
}

func (w *LogWriter) rotateFile() {
	if err := w.writer.Flush(); err != nil {
		slog.Error("Failed to flush before rotate", "error", err)
	}
	if err := w.file.Close(); err != nil {
		slog.Error("Failed to close file before rotate", "error", err)
	}

	newPath := fmt.Sprintf("%s.%s", w.path, time.Now().Format("20060102-150405"))
	os.Rename(w.path, newPath)

	var err error
	w.file, err = os.Create(w.path)
	if err != nil {
		slog.Error("Failed to create new log file", "error", err)
		panic(err) // 无法创建新文件，严重错误
	}
	w.writer = bufio.NewWriterSize(w.file, 32*1024) // 重新创建缓冲写入器
	w.currentSize = 0
}
