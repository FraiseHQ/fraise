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
	"io"
	"os"
	"strings"
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

// captureStdout swaps os.Stdout for a pipe while fn runs and returns what was
// written to it. The handler captures os.Stdout when it is built, so fn has to
// construct the logger too, not just log through one.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = original }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("closing pipe: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading captured output: %v", err)
	}
	return string(out)
}

// TestNewLogger checks that a logger is constructed for every supported level
// and format, and that a config which never went through config.Parse — so
// carries an unset or unrecognised value — still yields a working logger
// instead of panicking. Startup rejects unrecognised values before they reach
// here; these rows only pin that the fallback is a logger, not a crash.
func TestNewLogger(t *testing.T) {
	cases := []struct {
		level  string
		format string
	}{
		{config.LogLevelDebug, config.LogFormatText},
		{config.LogLevelInfo, config.LogFormatJSON},
		{config.LogLevelWarn, config.LogFormatText},
		{config.LogLevelError, config.LogFormatJSON},
		{"", ""},               // zero-value config: the fallback's real audience
		{"UNKNOWN", "UNKNOWN"}, // never reaches here via Parse; must not panic
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

// TestLevelFiltersLowerSeverities is the bug report as a test: the configured
// level must decide what comes out. The reported symptom was starting with
// -log-level error and still seeing INFO lines, because the level never made it
// past a case-sensitive switch — a logger that returns a valid *Logger for any
// input, which is all the older tests here checked, is exactly what let that
// through.
//
// Configs are built with the canonical constants because that is what
// config.Parse hands NewLogger; the casing an operator types is the config
// package's problem, and is pinned there.
func TestLevelFiltersLowerSeverities(t *testing.T) {
	cases := []struct {
		level  string
		want   []string
		unwant []string
		format string
	}{
		{config.LogLevelDebug, []string{"dbg", "inf", "wrn", "err"}, nil, config.LogFormatText},
		{config.LogLevelInfo, []string{"inf", "wrn", "err"}, []string{"dbg"}, config.LogFormatText},
		{config.LogLevelWarn, []string{"wrn", "err"}, []string{"dbg", "inf"}, config.LogFormatText},
		{config.LogLevelError, []string{"err"}, []string{"dbg", "inf", "wrn"}, config.LogFormatJSON},
	}

	for _, tc := range cases {
		t.Run(tc.level, func(t *testing.T) {
			out := captureStdout(t, func() {
				l := logger.NewLogger(newConfig(tc.level, tc.format))
				l.Debug("dbg")
				l.Info("inf")
				l.Warn("wrn")
				l.Error("err")
			})

			for _, msg := range tc.want {
				if !strings.Contains(out, msg) {
					t.Errorf("level %s dropped %q, which is at or above it:\n%s", tc.level, msg, out)
				}
			}
			for _, msg := range tc.unwant {
				if strings.Contains(out, msg) {
					t.Errorf("level %s emitted %q, which is below it:\n%s", tc.level, msg, out)
				}
			}
		})
	}
}

// TestFormatSelectsHandler pins that both accepted formats reach a handler of
// their own. "text" is the one that matters: it is the default, and it used to
// be absent from the switch, working only because it fell through to the same
// handler the default arm happened to build — so the default was never
// exercised as a value, and a typo in it would have gone unnoticed.
func TestFormatSelectsHandler(t *testing.T) {
	cases := []struct {
		format string
		want   string // a fragment only this encoding produces
	}{
		{config.LogFormatText, `msg=hello`},
		{config.LogFormatJSON, `"msg":"hello"`},
	}

	for _, tc := range cases {
		t.Run(tc.format, func(t *testing.T) {
			out := captureStdout(t, func() {
				logger.NewLogger(newConfig(config.LogLevelInfo, tc.format)).Info("hello")
			})

			if !strings.Contains(out, tc.want) {
				t.Errorf("format %q did not produce %s output, got:\n%s", tc.format, tc.format, out)
			}
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
