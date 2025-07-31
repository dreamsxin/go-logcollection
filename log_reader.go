package logcollection

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"google.golang.org/protobuf/proto"
)

const (
	ReadStateFileName = "read_state.json"
)

type LogReader struct {
	rootDir   string
	stateFile string
	mu        sync.Mutex
}

type readState struct {
	FilePath string `json:"filePath"`
	Offset   int64  `json:"offset"`
}

func NewLogReader(dir string) *LogReader {
	rootDir := filepath.Clean(dir)
	_, err := os.Stat(rootDir)
	if err != nil {
		slog.Error("ReadAll", "msg", "目录无法读取", "err", err)
		panic(err)
	}
	return &LogReader{
		rootDir:   rootDir,
		stateFile: filepath.Join(rootDir, ReadStateFileName),
	}
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

func (r *LogReader) SaveState(filePath string, offset int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	state := readState{
		FilePath: filePath,
		Offset:   offset,
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(r.stateFile, data, 0644)
}

func (r *LogReader) ReadAllOld(maxLine int64, callback func(*LogEntry)) error {

	lastfile := ""
	offset := int64(0)

	_, err := os.Stat(r.stateFile)
	if err != nil {
		slog.Error("ReadAll", "msg", "目录无法读取", "err", err)
	} else {
		// 加载上次读取位置
		lastfile, offset, err = r.LoadState()
		if err != nil {
			if err != os.ErrNotExist {
				slog.Error("ReadAll", "msg", "加载上次读取位置失败", "err", err)
				return err
			}
		}
	}
	slog.Debug("ReadAll", "msg", "加载上次读取位置", "lastfile", lastfile, "offset", offset)
	linenum := int64(0)
	filepath.Walk(r.rootDir, func(path string, info os.FileInfo, err error) error {
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
		file, _ := os.Open(path)
		defer file.Close()

		// 加载上次读取位置
		slog.Debug("ReadAll", "msg", "遍历目录", "path", path, "lastfile", lastfile, "offset", offset)
		if path == lastfile {
			if offset > 0 {
				file.Seek(offset, io.SeekStart)
				slog.Debug("ReadAll", "Seek", offset)
			}
		}

		// 按行读取
		reader := bufio.NewReader(file)
		for {
			line, _, err := reader.ReadLine()
			if err == io.EOF {
				break
			}

			r.processChunk(line, callback)

			// 保存当前读取位置
			offset += int64(len(line)) + 1
			r.SaveState(path, offset)

			linenum++
			if maxLine > 0 && linenum >= maxLine {
				return nil
			}
		}
		return nil
	})

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

func (r *LogReader) Read(maxLine int64, callback func(*LogEntry)) error {

	lastfile := ""
	offset := int64(0)

	_, err := os.Stat(r.stateFile)
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
	filepath.Walk(r.rootDir, func(path string, info os.FileInfo, err error) error {
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
		file, _ := os.Open(path)
		defer file.Close()

		// 加载上次读取位置
		slog.Debug("ReadAll", "msg", "遍历目录", "path", path, "lastfile", lastfile, "offset", offset)
		if path == lastfile {
			if offset > 0 {
				file.Seek(offset, io.SeekStart)
			}
		}

		// 按行读取
		scanner := bufio.NewScanner(file)
		buf := make([]byte, 0, 1024*1024) // 1MB初始缓冲区
		scanner.Buffer(buf, 10*1024*1024) // 最大10MB

		scanner.Split(splitProtobuf)
		for scanner.Scan() {
			msg := scanner.Bytes()

			r.processChunk(msg, callback)

			// 保存当前读取位置
			offset += int64(len(msg)) + 4
			r.SaveState(path, offset)

			linenum++
			if maxLine > 0 && linenum >= maxLine {
				return nil
			}
		}
		return nil
	})

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
