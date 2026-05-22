package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// LogStyle defines a log message style
type LogStyle struct {
	Prefix      string
	Color       string
	Icon        string
	Level       LogLevel
	IsHeader    bool
	IsSubHeader bool
	IsInfo      bool
	IsSuccess   bool
	IsWarning   bool
	IsError     bool
}

// ANSI color codes
const (
	ColorReset   = "\033[0m"
	ColorRed     = "\033[31m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorMagenta = "\033[35m"
	ColorCyan    = "\033[36m"
	ColorWhite   = "\033[37m"
	ColorGray    = "\033[90m"

	// Bright colors
	ColorBrightGreen   = "\033[92m"
	ColorBrightYellow  = "\033[93m"
	ColorBrightBlue    = "\033[94m"
	ColorBrightMagenta = "\033[95m"
	ColorBrightCyan    = "\033[96m"

	// Background colors
	BgBlue    = "\033[44m"
	BgMagenta = "\033[45m"
	BgCyan    = "\033[46m"
)

// Predefined styles
var (
	styleHeader = LogStyle{
		Icon:     "╔═══════════════════════════════════════════════════════════╗",
		Level:    LevelInfo,
		IsHeader: true,
	}
	styleSubHeader = LogStyle{
		Icon:        "───────────────────────────────────────────────────────",
		Level:       LevelInfo,
		IsSubHeader: true,
	}
	styleNetwork = LogStyle{
		Icon:   "🌐",
		Color:  ColorBrightCyan,
		Level:  LevelInfo,
		Prefix: "NETWORK",
	}
	styleConsensus = LogStyle{
		Icon:   "⛏️",
		Color:  ColorBrightMagenta,
		Level:  LevelInfo,
		Prefix: "CONSENSUS",
	}
	styleMining = LogStyle{
		Icon:   "🔨",
		Color:  ColorBrightYellow,
		Level:  LevelInfo,
		Prefix: "MINING",
	}
	styleBlockProduced = LogStyle{
		Icon:   "✅",
		Color:  ColorBrightGreen,
		Level:  LevelInfo,
		Prefix: "BLOCK PRODUCED",
	}
	styleValidation = LogStyle{
		Icon:   "🔍",
		Color:  ColorBrightCyan,
		Level:  LevelInfo,
		Prefix: "VALIDATION",
	}
	styleSync = LogStyle{
		Icon:   "🔄",
		Color:  ColorBlue,
		Level:  LevelDebug,
		Prefix: "SYNC",
	}
	styleConnection = LogStyle{
		Icon:   "🔗",
		Color:  ColorGreen,
		Level:  LevelDebug,
		Prefix: "CONNECTION",
	}
	styleP2P = LogStyle{
		Icon:   "📡",
		Color:  ColorCyan,
		Level:  LevelDebug,
		Prefix: "P2P",
	}
	styleHTTP = LogStyle{
		Icon:   "🌍",
		Color:  ColorGreen,
		Level:  LevelInfo,
		Prefix: "HTTP",
	}
	styleMetrics = LogStyle{
		Icon:   "📊",
		Color:  ColorMagenta,
		Level:  LevelInfo,
		Prefix: "METRICS",
	}
	styleSecurity = LogStyle{
		Icon:   "🔒",
		Color:  ColorBrightGreen,
		Level:  LevelInfo,
		Prefix: "SECURITY",
	}
	styleConfig = LogStyle{
		Icon:   "⚙️",
		Color:  ColorWhite,
		Level:  LevelInfo,
		Prefix: "CONFIG",
	}
	styleInfo = LogStyle{
		Icon:    "ℹ️",
		Color:   ColorCyan,
		Level:   LevelInfo,
		Prefix:  "INFO",
		IsInfo:  true,
	}
	styleSuccess = LogStyle{
		Icon:      "✅",
		Color:     ColorBrightGreen,
		Level:     LevelInfo,
		Prefix:    "SUCCESS",
		IsSuccess: true,
	}
	styleWarning = LogStyle{
		Icon:      "⚠️",
		Color:     ColorBrightYellow,
		Level:     LevelWarn,
		Prefix:    "WARNING",
		IsWarning: true,
	}
	styleError = LogStyle{
		Icon:    "❌",
		Color:   ColorRed,
		Level:   LevelError,
		Prefix:  "ERROR",
		IsError: true,
	}
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

func parseLogLevel(s string) LogLevel {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// LogFormatter provides structured logging with visual formatting
type LogFormatter struct {
	useColors bool
	level     LogLevel
	logger    *log.Logger
}

// NewLogFormatter creates a new log formatter
func NewLogFormatter(useColors bool) *LogFormatter {
	levelStr := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL")))
	if levelStr == "" {
		levelStr = "info"
	}
	return &LogFormatter{
		useColors: useColors,
		level:     parseLogLevel(levelStr),
		logger:    log.New(os.Stdout, "", 0),
	}
}

// SetLevel changes the minimum log level dynamically
func (f *LogFormatter) SetLevel(level LogLevel) {
	f.level = level
}

// Level returns the current log level
func (f *LogFormatter) Level() LogLevel {
	return f.level
}

// colorize applies color to text if colors are enabled
func (f *LogFormatter) colorize(text string, color string) string {
	if !f.useColors {
		return text
	}
	return color + text + ColorReset
}

// log writes a message at the given level, respecting the configured log level
func (f *LogFormatter) log(level LogLevel, format string, args ...interface{}) {
	if level < f.level {
		return
	}
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf(format, args...)
	switch level {
	case LevelDebug:
		f.logger.Printf("%s %s[DEBUG] %s\n", timestamp, f.colorize("🔍", ColorGray), f.colorize(message, ColorGray))
	case LevelWarn:
		f.logger.Printf("%s %s[WARN] %s\n", timestamp, f.colorize("⚠️", ColorBrightYellow), f.colorize(message, ColorBrightYellow))
	case LevelError:
		f.logger.Printf("%s %s[ERROR] %s\n", timestamp, f.colorize("❌", ColorRed), f.colorize(message, ColorRed))
	default:
		f.logger.Printf("%s %s[INFO] %s\n", timestamp, f.colorize("ℹ️", ColorCyan), message)
	}
}

// PrintHeader prints a main header
func (f *LogFormatter) PrintHeader(title string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	border := styleHeader.Icon

	f.logger.Println(f.colorize(border, ColorBrightCyan))
	f.logger.Printf("%s %s\n", timestamp, f.colorize(title, ColorBrightCyan))
	f.logger.Println(f.colorize(border, ColorBrightCyan))
}

// PrintSubHeader prints a section header
func (f *LogFormatter) PrintSubHeader(section string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	divider := styleSubHeader.Icon

	f.logger.Println(f.colorize(divider, ColorGray))
	f.logger.Printf("%s %s\n", timestamp, f.colorize(section, ColorBrightBlue))
}

// PrintStyled prints a styled log message, respecting the configured log level
func (f *LogFormatter) PrintStyled(style LogStyle, format string, args ...interface{}) {
	// Skip if below current log level
	if style.Level < f.level {
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf(format, args...)

	var logLine strings.Builder

	logLine.WriteString(timestamp)
	logLine.WriteString(" ")

	if style.Icon != "" {
		logLine.WriteString(style.Icon)
		logLine.WriteString(" ")
	}

	if style.Prefix != "" {
		logLine.WriteString(f.colorize("[", style.Color))
		logLine.WriteString(f.colorize(style.Prefix, style.Color))
		logLine.WriteString(f.colorize("]", style.Color))
		logLine.WriteString(" ")
	}

	if style.Color != "" {
		logLine.WriteString(f.colorize(message, style.Color))
	} else {
		logLine.WriteString(message)
	}

	f.logger.Println(logLine.String())
}

// Convenience methods
func (f *LogFormatter) Debug(format string, args ...interface{}) {
	f.log(LevelDebug, format, args...)
}

func (f *LogFormatter) Network(format string, args ...interface{}) {
	f.PrintStyled(styleNetwork, format, args...)
}

func (f *LogFormatter) Consensus(format string, args ...interface{}) {
	f.PrintStyled(styleConsensus, format, args...)
}

func (f *LogFormatter) Mining(format string, args ...interface{}) {
	f.PrintStyled(styleMining, format, args...)
}

func (f *LogFormatter) BlockProduced(format string, args ...interface{}) {
	f.PrintStyled(styleBlockProduced, format, args...)
}

func (f *LogFormatter) Validation(format string, args ...interface{}) {
	f.PrintStyled(styleValidation, format, args...)
}

func (f *LogFormatter) Sync(format string, args ...interface{}) {
	f.PrintStyled(styleSync, format, args...)
}

func (f *LogFormatter) Connection(format string, args ...interface{}) {
	f.PrintStyled(styleConnection, format, args...)
}

func (f *LogFormatter) P2P(format string, args ...interface{}) {
	f.PrintStyled(styleP2P, format, args...)
}

func (f *LogFormatter) HTTP(format string, args ...interface{}) {
	f.PrintStyled(styleHTTP, format, args...)
}

func (f *LogFormatter) Metrics(format string, args ...interface{}) {
	f.PrintStyled(styleMetrics, format, args...)
}

func (f *LogFormatter) Security(format string, args ...interface{}) {
	f.PrintStyled(styleSecurity, format, args...)
}

func (f *LogFormatter) Config(format string, args ...interface{}) {
	f.PrintStyled(styleConfig, format, args...)
}

func (f *LogFormatter) Info(format string, args ...interface{}) {
	f.PrintStyled(styleInfo, format, args...)
}

func (f *LogFormatter) Success(format string, args ...interface{}) {
	f.PrintStyled(styleSuccess, format, args...)
}

func (f *LogFormatter) Warning(format string, args ...interface{}) {
	f.PrintStyled(styleWarning, format, args...)
}

func (f *LogFormatter) Error(format string, args ...interface{}) {
	f.PrintStyled(styleError, format, args...)
}

// Printf prints a formatted message using the standard logger
func (f *LogFormatter) Printf(format string, args ...interface{}) {
	f.logger.Printf(format, args...)
}

// Println prints a message using the standard logger
func (f *LogFormatter) Println(args ...interface{}) {
	f.logger.Println(args...)
}

// Global formatter instance
var globalFormatter = NewLogFormatter(true)

// SetGlobalFormatter sets the global formatter with color preference
func SetGlobalFormatter(useColors bool) {
	globalFormatter = NewLogFormatter(useColors)
}

// GetGlobalFormatter returns the global formatter
func GetGlobalFormatter() *LogFormatter {
	return globalFormatter
}
