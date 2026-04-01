package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	case LevelFatal:
		return "fatal"
	default:
		return "unknown"
	}
}

type Logger struct {
	mu      sync.Mutex
	level   Level
	out     io.Writer
	appName string
}

type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	App       string                 `json:"app,omitempty"`
	Message   string                 `json:"message"`
	Caller    string                 `json:"caller,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

func NewLogger(appName string, level Level) *Logger {
	return &Logger{
		level:   level,
		out:     os.Stdout,
		appName: appName,
	}
}

func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.out = w
}

func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

func (l *Logger) log(level Level, msg string, fields map[string]interface{}) {
	if level < l.level {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     level.String(),
		Message:   msg,
		Fields:    fields,
	}

	if l.appName != "" {
		entry.App = l.appName
	}

	_, file, line, ok := runtime.Caller(2)
	if ok {
		entry.Caller = fmt.Sprintf("%s:%d", file, line)
	}

	data, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(l.out, "{\"error\":\"json marshal failed: %v\"}\n", err)
		return
	}

	l.mu.Lock()
	fmt.Fprintln(l.out, string(data))
	l.mu.Unlock()
}

func (l *Logger) Debug(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelDebug, msg, f)
}

func (l *Logger) Info(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelInfo, msg, f)
}

func (l *Logger) Warn(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelWarn, msg, f)
}

func (l *Logger) Error(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelError, msg, f)
}

func (l *Logger) Fatal(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelFatal, msg, f)
	os.Exit(1)
}

func (l *Logger) WithField(key string, value interface{}) map[string]interface{} {
	return map[string]interface{}{key: value}
}

func (l *Logger) WithFields(fields map[string]interface{}) map[string]interface{} {
	return fields
}

var globalLogger = NewLogger("nogo", LevelInfo)

func SetGlobalLogger(logger *Logger) {
	globalLogger = logger
}

func Debug(msg string, fields ...map[string]interface{}) {
	globalLogger.Debug(msg, fields...)
}

func Info(msg string, fields ...map[string]interface{}) {
	globalLogger.Info(msg, fields...)
}

func Warn(msg string, fields ...map[string]interface{}) {
	globalLogger.Warn(msg, fields...)
}

func Error(msg string, fields ...map[string]interface{}) {
	globalLogger.Error(msg, fields...)
}

func Fatal(msg string, fields ...map[string]interface{}) {
	globalLogger.Fatal(msg, fields...)
}
