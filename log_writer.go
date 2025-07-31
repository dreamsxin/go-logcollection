package logcollection

import (
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

type LogWriter struct {
	path        string
	file        *os.File
	currentSize int64
	maxSize     int64
	mu          sync.Mutex
	buffer      chan proto.Message
	stopChan    chan struct{}
	wg          sync.WaitGroup
	closed      bool
}

func NewLogWriter(path string, maxSizeMB int, bufferSize int) *LogWriter {
	dir := filepath.Dir(path)
	_, err := os.Stat(dir)
	if err != nil {
		os.MkdirAll(dir, os.ModePerm)
	}

	file, _ := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	w := &LogWriter{
		path:     path,
		file:     file,
		maxSize:  int64(maxSizeMB) * 1024 * 1024,
		buffer:   make(chan proto.Message, bufferSize),
		stopChan: make(chan struct{}),
	}

	w.wg.Add(1)
	go w.batchWriter()
	return w
}

func (w *LogWriter) Write(entry proto.Message) error {
	if w.closed {
		return fmt.Errorf("writer is closed")
	}
	w.buffer <- entry
	return nil
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

	var batch []byte
	ticker := time.NewTicker(100 * time.Millisecond)

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
			if len(batch) > 4*1024 {
				w.flush(batch)
				batch = nil
			}
		case <-ticker.C:
			if len(batch) > 0 {
				w.flush(batch)
				batch = nil
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
	w.mu.Lock()
	defer w.mu.Unlock()

	w.file.Write(data)
	w.currentSize += int64(len(data))
	if w.currentSize >= w.maxSize {
		w.rotateFile()
	}
}

func (w *LogWriter) rotateFile() {
	w.file.Close()
	newPath := fmt.Sprintf("%s.%s", w.path, time.Now().Format("20060102-150405"))
	os.Rename(w.file.Name(), newPath)
	w.file, _ = os.Create(w.file.Name())
	w.currentSize = 0
}
