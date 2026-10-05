/* ----- ----- ----- ----- */
// logger.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2026/10/05
// Version: v1.1
/* ----- ----- ----- ----- */

package logger

import (
	"fmt"
	"time"
)

// ANSI terminal colours for LogfColor; ColorReset restores the terminal's own colour.
const (
	ColorReset   = "\033[0m"
	ColorBlack   = "\033[30m"
	ColorRed     = "\033[31m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorMagenta = "\033[35m"
	ColorCyan    = "\033[36m"
	ColorWhite   = "\033[37m"
)

// Level is a log line's severity, shown in brackets after the time.
type Level string

const (
	INFO  Level = "INFO"
	WARN  Level = "WARN"
	ERROR Level = "ERROR"
)

// Logf prints a timestamped line at level, in the level's colour (INFO white, WARN yellow, ERROR red).
func Logf(level Level, format string, args ...interface{}) {
	LogfColor(level, "", format, args...)
}

// LogfColor prints a timestamped line at level in color (empty: the level's colour).
func LogfColor(level Level, color string, format string, args ...interface{}) {
	if color == "" {
		switch level {
		case INFO:
			color = ColorWhite
		case WARN:
			color = ColorYellow
		case ERROR:
			color = ColorRed
		default:
			color = ColorWhite
		}
	}

	msg := fmt.Sprintf(format, args...)
	t := time.Now().Format("15:04:05")

	output := fmt.Sprintf("[%s]", t)
	if level != "" {
		output += fmt.Sprintf(" [%s]", level)
	}
	output += " " + msg

	// Reset afterwards, so the terminal's own colour comes back.
	fmt.Println(color + output + ColorReset)
}

// Infof logs at INFO.
func Infof(format string, args ...interface{}) {
	Logf(INFO, format, args...)
}

// Warnf logs at WARN.
func Warnf(format string, args ...interface{}) {
	Logf(WARN, format, args...)
}

// Errorf logs at ERROR (it does not exit).
func Errorf(format string, args ...interface{}) {
	Logf(ERROR, format, args...)
}

// InfofColor logs at INFO in color.
func InfofColor(color string, format string, args ...interface{}) {
	LogfColor(INFO, color, format, args...)
}

// WarnfColor logs at WARN in color.
func WarnfColor(color string, format string, args ...interface{}) {
	LogfColor(WARN, color, format, args...)
}

// ErrorfColor logs at ERROR in color.
func ErrorfColor(color string, format string, args ...interface{}) {
	LogfColor(ERROR, color, format, args...)
}

// --- Option Pattern ---
// Not used yet: options for a future configurable log call (nothing reads logConfig).
type logConfig struct {
	level string
	color string
}

// Option sets one field of a logConfig.
type Option func(*logConfig)

// WithLevel sets the level.
func WithLevel(level Level) Option {
	return func(cfg *logConfig) {
		cfg.level = string(level)
	}
}

// WithColor sets the colour.
func WithColor(color string) Option {
	return func(cfg *logConfig) {
		cfg.color = color
	}
}
