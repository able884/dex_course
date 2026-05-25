package slot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// FailedSlotLog 失败 slot 日志结构
type FailedSlotLog struct {
	Timestamp    string `json:"timestamp"`
	Slot         uint64 `json:"slot"`
	Reason       string `json:"reason"`        // 失败原因（简短）
	ErrorMessage string `json:"error_message"` // 完整错误信息
	RetryCount   int    `json:"retry_count"`   // 重试次数
	Source       string `json:"source"`        // 来源：websocket 或 recovery
}

// FailedSlotLogger 失败 slot 日志记录器
// 职责：将重试多次后仍失败的 slot 记录到日志文件
type FailedSlotLogger struct {
	logDir string
	logger logx.Logger
	mu     sync.Mutex

	// 当前日志文件
	currentFile     *os.File
	currentFilePath string
	currentDate     string // 格式: 2006-01-02
}

// NewFailedSlotLogger 创建失败 slot 日志记录器
func NewFailedSlotLogger(logDir string) *FailedSlotLogger {
	return &FailedSlotLogger{
		logDir: logDir,
		logger: logx.WithContext(nil).WithFields(logx.Field("component", "failed_slot_logger")),
	}
}

// LogFailedSlot 记录失败的 slot
func (l *FailedSlotLogger) LogFailedSlot(slot uint64, reason string, errorMessage string, retryCount int, source string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 1. 准备日志数据
	logData := FailedSlotLog{
		Timestamp:    time.Now().Format(time.RFC3339),
		Slot:         slot,
		Reason:       reason,
		ErrorMessage: errorMessage,
		RetryCount:   retryCount,
		Source:       source,
	}

	// 2. 序列化为 JSON（单行）
	data, err := json.Marshal(logData)
	if err != nil {
		return fmt.Errorf("marshal log data failed: %w", err)
	}

	// 3. 确保日志文件打开
	if err := l.ensureLogFile(); err != nil {
		return fmt.Errorf("ensure log file failed: %w", err)
	}

	// 4. 写入文件（每行一个 JSON 对象）
	if _, err := l.currentFile.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write log file failed: %w", err)
	}

	// 5. 立即刷新到磁盘
	if err := l.currentFile.Sync(); err != nil {
		return fmt.Errorf("sync log file failed: %w", err)
	}

	return nil
}

// ensureLogFile 确保日志文件打开（按日期切分）
func (l *FailedSlotLogger) ensureLogFile() error {
	// 获取当前日期
	today := time.Now().Format("2006-01-02")

	// 如果日期变化或文件未打开，创建新文件
	if l.currentFile == nil || l.currentDate != today {
		// 关闭旧文件
		if l.currentFile != nil {
			l.currentFile.Close()
		}

		// 创建目录
		if err := os.MkdirAll(l.logDir, 0755); err != nil {
			return fmt.Errorf("create log directory failed: %w", err)
		}

		// 生成文件名：failed_slots_2026-01-01.jsonl
		filename := fmt.Sprintf("failed_slots_%s.jsonl", today)
		fullPath := filepath.Join(l.logDir, filename)

		// 以追加模式打开文件
		file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("open log file failed: %w", err)
		}

		l.currentFile = file
		l.currentFilePath = fullPath
		l.currentDate = today

		l.logger.Infof("Opened failed slot log file: %s", fullPath)
	}

	return nil
}

// Close 关闭日志文件
func (l *FailedSlotLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.currentFile != nil {
		if err := l.currentFile.Close(); err != nil {
			return fmt.Errorf("close log file failed: %w", err)
		}
		l.currentFile = nil
		l.logger.Infof("Closed failed slot log file: %s", l.currentFilePath)
	}

	return nil
}
