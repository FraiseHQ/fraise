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

package logger_test

import (
	"testing"

	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/pkg/logger"
)

// newConfig returns a config whose log level/format can be overridden for a
// single test case.
func newConfig(level, format string) *config.ConfigSet {
	c := config.New()
	c.Log.Level = level
	c.Log.Format = format
	return c
}

// TestNewLogger checks that a logger is constructed for every supported level
// and format, including unknown values which fall back to sensible defaults.
func TestNewLogger(t *testing.T) {
	cases := []struct {
		level  string
		format string
	}{
		{"DEBUG", "console"},
		{"INFO", "json"},
		{"WARN", "console"},
		{"ERROR", "json"},
		{"UNKNOWN", "UNKNOWN"}, // both fall back to defaults
	}

	for _, tc := range cases {
		t.Run(tc.level+"/"+tc.format, func(t *testing.T) {
			l := logger.NewLogger(newConfig(tc.level, tc.format))
			if l == nil {
				t.Fatal("NewLogger returned nil")
			}
			// The methods must not panic for any level/format combination.
			l.Debug("debug", "k", "v")
			l.Info("info", "k", "v")
			l.Warn("warn", "k", "v")
			l.Error("error", "k", "v")
		})
	}
}

// TestSetDefaultRoundTrip checks that SetDefault installs the logger that
// Default subsequently returns.
func TestSetDefaultRoundTrip(t *testing.T) {
	l := logger.NewLogger(newConfig("INFO", "json"))
	logger.SetDefault(l)
	if got := logger.Default(); got != l {
		t.Errorf("Default() = %p, want %p", got, l)
	}
}

// TestPackageFuncsNilDefault checks that the package-level helpers are no-ops
// (and do not panic) when no default logger has been installed.
func TestPackageFuncsNilDefault(t *testing.T) {
	logger.SetDefault(nil)
	// None of these should panic with a nil default logger.
	logger.Debug("debug", "k", "v")
	logger.Info("info", "k", "v")
	logger.Warn("warn", "k", "v")
	logger.Error("error", "k", "v")
}

// TestPackageFuncsWithDefault checks that the package-level helpers dispatch to
// the installed default logger without panicking.
func TestPackageFuncsWithDefault(t *testing.T) {
	logger.SetDefault(logger.NewLogger(newConfig("DEBUG", "json")))
	t.Cleanup(func() { logger.SetDefault(nil) })

	logger.Debug("debug", "k", "v")
	logger.Info("info", "k", "v")
	logger.Warn("warn", "k", "v")
	logger.Error("error", "k", "v")
}
