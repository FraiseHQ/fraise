// MIT License

// Copyright (c) 2026 René-Jean Corneille

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package logger

import (
	"context"
	"os"

	"github.com/RonsenbergVI/fraise/internal/config"
	"golang.org/x/exp/slog"
)

type Logger struct {
	config *config.ConfigSet
	logger *slog.Logger
}

var defaultLogger *Logger

func NewLogger(config *config.ConfigSet) *Logger {

	var handler slog.Handler
	var level slog.Level

	// set logging level
	switch config.Log.Level {
	case "DEBUG":
		level = slog.LevelDebug
	case "INFO":
		level = slog.LevelInfo
	case "WARN":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	// set logging handler
	switch config.Log.Format {
	case "console":
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	default:
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	}

	l := &Logger{
		config: config,
		logger: slog.New(handler),
	}

	return l
}

func Default() *Logger {
	return defaultLogger
}

func SetDefault(l *Logger) {
	defaultLogger = l
}

func (l *Logger) log(level slog.Level, msg string, attrs ...any) {
	l.logger.Log(context.Background(), level, msg, attrs...)
}

func (l *Logger) Debug(msg string, attrs ...any) {
	l.log(slog.LevelDebug, msg, attrs...)
}

func (l *Logger) Info(msg string, attrs ...any) {
	l.log(slog.LevelInfo, msg, attrs...)
}

func (l *Logger) Warn(msg string, attrs ...any) {
	l.log(slog.LevelWarn, msg, attrs...)
}

func (l *Logger) Error(msg string, attrs ...any) {
	l.log(slog.LevelError, msg, attrs...)
}

func Debug(msg string, attrs ...any) {
	if defaultLogger == nil {
		return
	}
	defaultLogger.Debug(msg, attrs...)
}

func Info(msg string, attrs ...any) {
	if defaultLogger == nil {
		return
	}
	defaultLogger.Info(msg, attrs...)
}

func Warn(msg string, attrs ...any) {
	if defaultLogger == nil {
		return
	}
	defaultLogger.Warn(msg, attrs...)
}

func Error(msg string, attrs ...any) {
	if defaultLogger == nil {
		return
	}
	defaultLogger.Error(msg, attrs...)
}
