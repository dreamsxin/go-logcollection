package logcollection

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"google.golang.org/protobuf/proto"
)

const (
	ReadStateFileName = "read_state.json"
)

// 定义选项结构体
type LogReaderOptions struct {
	BufferSize    int
	MaxBufferSize int
	StateSaveFreq int    // 状态保存频率(条)
	MaxOpenFiles  int    // 最大打开文件数
	SortBy        string // 新增：支持 "modtime", "name"
}

// 选项函数
func WithReadBufferSize(size int) func(*LogReaderOptions) {
	return func(o *LogReaderOptions) { o.BufferSize = size }
}

func WithMaxOpenFiles(n int) func(*LogReaderOptions) {
	return func(o *LogReaderOptions) { o.MaxOpenFiles = n }
}

func WithSortBy(field string) func(*LogReaderOptions) {
	return func(o *LogReaderOptions) { o.SortBy = field }
}

type LogReader struct {
	rootDir   string
	stateFile string
	mu        sync.Mutex
	options   LogReaderOptions
	stateChan chan readState // 异步状态保存通道
	stopChan  chan struct{}  // 协程退出信号
	wg        sync.WaitGroup // 协程等待组
}

type readState struct {
	FilePath string `json:"filePath"`
	Offset   int64  `json:"offset"`
}

func NewLogReader(dir string, opts ...func(*LogReaderOptions)) (*LogReader, error) {
	// 设置默认选项
	options := LogReaderOptions{
		BufferSize:    1,
		MaxBufferSize: 10,
		StateSaveFreq: 1000,
		MaxOpenFiles:  1024,
	}

	// 应用用户提供的选项
	for _, opt := range opts {
		opt(&options)
	}

	rootDir := filepath.Clean(dir)
	_, err := os.Stat(rootDir)
	if err != nil {
		slog.Error("ReadAll", "msg", "目录无法读取", "err", err)
		return nil, err
	}

	lr := &LogReader{
		rootDir:   rootDir,
		stateFile: filepath.Join(rootDir, ReadStateFileName),
		options:   options,
		stateChan: make(chan readState, 100), // 缓冲100个状态更新
		stopChan:  make(chan struct{}),
	}
	// 启动状态保存协程
	lr.startStateSaver()
	return lr, nil
}

func (r *LogReader) LoadState() (string, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := os.ReadFile(r.stateFile)
	if err != nil {
		return "", 0, err
	}

	var state readState
	if err := json.Unmarshal(data, &state); err != nil {
		return "", 0, err
	}

	return state.FilePath, state.Offset, nil
}

// 启动状态保存协程
func (r *LogReader) startStateSaver() {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for {
			select {
			case state := <-r.stateChan:
				// 实际执行状态保存
				if err := r.saveStateToDisk(state); err != nil {
					slog.Error("状态保存失败", "err", err)
				}
			case <-r.stopChan:
				// 处理剩余状态
				for {
					select {
					case state := <-r.stateChan:
						if err := r.saveStateToDisk(state); err != nil {
							slog.Error("关闭时状态保存失败", "err", err)
						}
					default:
						return // 通道为空，退出
					}
				}
			}
		}
	}()
}

// 实际执行状态保存到磁盘的函数
func (r *LogReader) saveStateToDisk(state readState) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("序列化状态失败: %w", err)
	}

	if err := os.WriteFile(r.stateFile, data, 0644); err != nil {
		return fmt.Errorf("写入状态文件失败: %w", err)
	}
	return nil
}

// SaveState 将状态保存请求发送到异步通道
func (r *LogReader) saveState(filePath string, offset int64) error {
	if r.closed() {
		return fmt.Errorf("LogReader已关闭")
	}

	state := readState{
		FilePath: filePath,
		Offset:   offset,
	}

	select {
	case r.stateChan <- state:
		return nil
	default:
		// 通道满时的降级处理
		return r.saveStateToDisk(state)
	}
}

// 判断LogReader是否已关闭
func (r *LogReader) closed() bool {
	select {
	case <-r.stopChan:
		return true
	default:
		return false
	}
}

// Close 优雅关闭LogReader，确保所有状态保存完成
func (r *LogReader) Close() error {
	close(r.stopChan)
	r.wg.Wait()
	return nil
}

func splitProtobuf(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if len(data) < 4 { // 假设使用4字节长度前缀
		if atEOF { // 数据不完整且已到结尾
			slog.Error("splitProtobuf", "msg", "数据不完整且已到结尾", "len", len(data))
			return 0, nil, io.ErrUnexpectedEOF
		}
		return 0, nil, nil
	}
	length := binary.BigEndian.Uint32(data[:4])
	if uint32(len(data)) < 4+length {
		if atEOF { // 数据不完整且已到结尾
			slog.Error("splitProtobuf", "msg", "数据不完整且已到结尾", "len", len(data))
			return 0, nil, io.ErrUnexpectedEOF
		}
		return 0, nil, nil
	}
	return 4 + int(length), data[4 : 4+length], nil
}

func (r *LogReader) ReadAll(callback func(*LogEntry)) error {

	return r.Read(0, callback)
}

type FileInfo struct {
	Path string      // 文件完整路径
	Info os.FileInfo // 文件信息（包含修改时间等元数据）
}

func (r *LogReader) getSortedLogFiles() ([]FileInfo, error) {
	var files []FileInfo
	err := filepath.Walk(r.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			slog.Error("ReadAll", "msg", "遍历目录失败", "err", err)
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if info.Name() == ReadStateFileName {
			return nil
		}
		files = append(files, FileInfo{Path: path, Info: info})
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 按修改时间排序
	sort.Slice(files, func(i, j int) bool {
		return files[i].Info.ModTime().Before(files[j].Info.ModTime())
	})
	return files, nil
}

func (r *LogReader) Read(maxLine int64, callback func(*LogEntry)) error {

	lastfile := ""
	offset := int64(0)

	// 新增：获取并排序日志文件
	files, err := r.getSortedLogFiles()
	if err != nil {
		return fmt.Errorf("获取日志文件失败: %w", err)
	}

	_, err = os.Stat(r.stateFile)
	if err != nil {
		slog.Error("ReadAll", "msg", "目录无法读取", "err", err)
	} else {
		// 加载上次读取位置
		lastfile, offset, err = r.LoadState()
		if err != nil {
			slog.Error("ReadAll", "msg", "加载上次读取位置失败", "err", err)
		}
	}
	slog.Debug("ReadAll", "msg", "加载上次读取位置", "lastfile", lastfile, "offset", offset)
	linenum := int64(0)
	for _, file := range files {
		path := file.Path
		info := file.Info
		if info.IsDir() {
			continue
		}
		if info.Name() == ReadStateFileName {
			continue
		}
		file, _ := os.Open(path)
		defer file.Close()

		// 加载上次读取位置
		slog.Debug("ReadAll", "msg", "遍历目录", "path", path, "lastfile", lastfile, "offset", offset)
		if path == lastfile {
			if offset > 0 {
				file.Seek(offset, io.SeekStart)
			}
		}

		batchSize := 0
		// 按行读取
		scanner := bufio.NewScanner(file)
		buf := make([]byte, 0, r.options.BufferSize*1024*1024) // 1MB初始缓冲区
		scanner.Buffer(buf, r.options.MaxBufferSize*1024*1024) // 最大10MB

		scanner.Split(splitProtobuf)
		for scanner.Scan() {
			msg := scanner.Bytes()

			r.processChunk(msg, callback)

			// 保存当前读取位置
			offset += int64(len(msg)) + 4

			batchSize++
			if batchSize >= r.options.StateSaveFreq {
				if err := r.saveState(path, offset); err != nil {
					return fmt.Errorf("加载读取状态失败: %w", err) // 修改为返回错误
				} else {
					batchSize = 0
				}
			}

			linenum++
			if maxLine > 0 && linenum >= maxLine {
				return nil
			}
		}
		// 处理剩余记录
		if batchSize > 0 {
			r.saveState(path, offset)
		}
	}

	return nil
}

func (r *LogReader) processChunk(data []byte, callback func(*LogEntry)) {

	if len(data) == 0 {
		slog.Error("processChunk", "msg", "数据为空")
		return
	}

	entry := &LogEntry{}
	if err := proto.Unmarshal(data, entry); err != nil {
		slog.Error("processChunk", "msg", err)
		return
	}

	callback(entry)
	// size := proto.Size(entry)
	// if size >= len(data) {
	// 	break
	// }
	// data = data[size:]
}
