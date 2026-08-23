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

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FraiseHQ/fraise/internal/config"
)

// withArgs runs the binary's own command line for one test, restoring it after.
// A missing -config keeps the run off whatever fraise.config.toml happens to sit
// in the working directory, so the flags under test are the whole configuration.
func withArgs(t *testing.T, flags ...string) {
	t.Helper()

	missing := filepath.Join(t.TempDir(), "does-not-exist.toml")
	original := os.Args
	os.Args = append([]string{"fraise", "-config", missing}, flags...)
	t.Cleanup(func() { os.Args = original })
}

// TestRunRefusesToStartOnAnInvalidValue is the startup contract at the level
// that owns it: run is where a rejected setting becomes a non-zero exit, and
// main does nothing with the error but log it and pass it to os.Exit(1). The
// layer below (config.Parse) can only report the problem — deciding it is fatal
// happens here, and used to not happen at all: every config error was a warning,
// so the server started anyway with a value the operator never asked for.
//
// Nothing is listening when this returns, which is the point: run gives up
// before building a database or binding a port.
func TestRunRefusesToStartOnAnInvalidValue(t *testing.T) {
	cases := []struct {
		flag, value, setting string
	}{
		{"-log-level", "verbose", "log.level"},
		{"-log-format", "console", "log.format"},
		{"-precision", "floast32", "db.precision"},
		{"-hashing-function", "murmur", "db.hashing-function.name"},
		{"-search-algorithm", "dfs", "db.search-algorithm.name"},
		{"-ranking-algorithm", "hits", "db.ranking-algorithm.name"},
	}

	for _, tc := range cases {
		t.Run(tc.flag+"="+tc.value, func(t *testing.T) {
			withArgs(t, tc.flag, tc.value)

			err := run()

			if !errors.Is(err, config.ErrInvalidValue) {
				t.Fatalf("run() = %v, want an ErrInvalidValue so main exits non-zero", err)
			}
			if !strings.Contains(err.Error(), tc.setting) {
				t.Errorf("error %q does not name the setting %q", err, tc.setting)
			}
			if !strings.Contains(err.Error(), "accepted:") {
				t.Errorf("error %q does not list the accepted values", err)
			}
		})
	}
}

// TestRunAcceptsAnyCasingOfAValueThatIsAccepted is the other side of the same
// decision, and the reported bug: `-log-level error` is what an operator types,
// and it must not be what stops the server. run cannot be called for a value it
// accepts — it would block serving until a signal — so this asserts on the
// configuration run would have been handed, through the same Parse call it
// makes, which is as far as the question can be taken without booting.
func TestRunAcceptsAnyCasingOfAValueThatIsAccepted(t *testing.T) {
	cases := []struct{ flag, value string }{
		{"-log-level", "error"},
		{"-log-format", "Json"},
		{"-precision", "Float64"},
		{"-hashing-function", "T1ha"},
		{"-search-algorithm", "EXCESS"},
		{"-ranking-algorithm", "PageRank"},
	}

	for _, tc := range cases {
		t.Run(tc.flag+"="+tc.value, func(t *testing.T) {
			withArgs(t, tc.flag, tc.value)

			c := config.New()
			err := c.Parse(os.Args[1:])

			if errors.Is(err, config.ErrInvalidValue) {
				t.Fatalf("%s %s would stop startup: %v", tc.flag, tc.value, err)
			}
		})
	}
}

// TestRunSurvivesAMissingConfigFile pins the line between the two failures run
// distinguishes. A missing config file is reported but survivable — the
// defaults are a valid configuration, and containers ship without one — so it
// must not take the fatal branch, or making values fatal would have made the
// server unstartable without a file.
func TestRunSurvivesAMissingConfigFile(t *testing.T) {
	withArgs(t)

	c := config.New()
	err := c.Parse(os.Args[1:])

	if err == nil {
		t.Fatal("Parse with no config file = nil error, want the file failure reported")
	}
	if errors.Is(err, config.ErrInvalidValue) {
		t.Fatalf("a missing config file took the fatal branch: %v", err)
	}
}
