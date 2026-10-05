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

type Level string

const (
	INFO  Level = "INFO"
	WARN  Level = "WARN"
	ERROR Level = "ERROR"
)

func Logf(level Level, format string, args ...interface{}) {
	LogfColor(level, "", format, args...)
}

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

func Infof(format string, args ...interface{}) {
	Logf(INFO, format, args...)
}

func Warnf(format string, args ...interface{}) {
	Logf(WARN, format, args...)
}

func Errorf(format string, args ...interface{}) {
	Logf(ERROR, format, args...)
}

func InfofColor(color string, format string, args ...interface{}) {
	LogfColor(INFO, color, format, args...)
}

func WarnfColor(color string, format string, args ...interface{}) {
	LogfColor(WARN, color, format, args...)
}

func ErrorfColor(color string, format string, args ...interface{}) {
	LogfColor(ERROR, color, format, args...)
}

// --- Option Pattern ---
type logConfig struct {
	level string
	color string
}

type Option func(*logConfig)

func WithLevel(level Level) Option {
	return func(cfg *logConfig) {
		cfg.level = string(level)
	}
}

func WithColor(color string) Option {
	return func(cfg *logConfig) {
		cfg.color = color
	}
}
