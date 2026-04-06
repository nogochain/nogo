package metrics

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
		IsHeader: true,
	}
	styleSubHeader = LogStyle{
		Icon:        "───────────────────────────────────────────────────────",
		IsSubHeader: true,
	}
	styleNetwork = LogStyle{
		Icon:   "🌐",
		Color:  ColorBrightCyan,
		Prefix: "NETWORK",
	}
	styleConsensus = LogStyle{
		Icon:   "⛏️",
		Color:  ColorBrightMagenta,
		Prefix: "CONSENSUS",
	}
	styleMining = LogStyle{
		Icon:   "🔨",
		Color:  ColorBrightYellow,
		Prefix: "MINING",
	}
	styleBlockProduced = LogStyle{
		Icon:   "✅",
		Color:  ColorBrightGreen,
		Prefix: "BLOCK PRODUCED",
	}
	styleValidation = LogStyle{
		Icon:   "🔍",
		Color:  ColorBrightCyan,
		Prefix: "VALIDATION",
	}
	styleSync = LogStyle{
		Icon:   "🔄",
		Color:  ColorBlue,
		Prefix: "SYNC",
	}
	styleConnection = LogStyle{
		Icon:   "🔗",
		Color:  ColorGreen,
		Prefix: "CONNECTION",
	}
	styleP2P = LogStyle{
		Icon:   "📡",
		Color:  ColorCyan,
		Prefix: "P2P",
	}
	styleHTTP = LogStyle{
		Icon:   "🌍",
		Color:  ColorGreen,
		Prefix: "HTTP",
	}
	styleMetrics = LogStyle{
		Icon:   "📊",
		Color:  ColorMagenta,
		Prefix: "METRICS",
	}
	styleSecurity = LogStyle{
		Icon:   "🔒",
		Color:  ColorBrightGreen,
		Prefix: "SECURITY",
	}
	styleConfig = LogStyle{
		Icon:   "⚙️",
		Color:  ColorWhite,
		Prefix: "CONFIG",
	}
	styleInfo = LogStyle{
		Icon:   "ℹ️",
		Color:  ColorCyan,
		Prefix: "INFO",
		IsInfo: true,
	}
	styleSuccess = LogStyle{
		Icon:      "✅",
		Color:     ColorBrightGreen,
		Prefix:    "SUCCESS",
		IsSuccess: true,
	}
	styleWarning = LogStyle{
		Icon:      "⚠️",
		Color:     ColorBrightYellow,
		Prefix:    "WARNING",
		IsWarning: true,
	}
	styleError = LogStyle{
		Icon:    "❌",
		Color:   ColorRed,
		Prefix:  "ERROR",
		IsError: true,
	}
)

// LogFormatter provides structured logging with visual formatting
type LogFormatter struct {
	useColors bool
	logger    *log.Logger
}

// NewLogFormatter creates a new log formatter
func NewLogFormatter(useColors bool) *LogFormatter {
	return &LogFormatter{
		useColors: useColors,
		logger:    log.New(os.Stdout, "", 0),
	}
}

// colorize applies color to text if colors are enabled
func (f *LogFormatter) colorize(text string, color string) string {
	if !f.useColors {
		return text
	}
	return color + text + ColorReset
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

// PrintStyled prints a styled log message
func (f *LogFormatter) PrintStyled(style LogStyle, format string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf(format, args...)

	// Build the log line
	var logLine strings.Builder

	// Timestamp
	logLine.WriteString(timestamp)
	logLine.WriteString(" ")

	// Icon
	if style.Icon != "" {
		logLine.WriteString(style.Icon)
		logLine.WriteString(" ")
	}

	// Prefix
	if style.Prefix != "" {
		logLine.WriteString(f.colorize("[", style.Color))
		logLine.WriteString(f.colorize(style.Prefix, style.Color))
		logLine.WriteString(f.colorize("]", style.Color))
		logLine.WriteString(" ")
	}

	// Message
	if style.Color != "" {
		logLine.WriteString(f.colorize(message, style.Color))
	} else {
		logLine.WriteString(message)
	}

	f.logger.Println(logLine.String())
}

// Convenience methods
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
