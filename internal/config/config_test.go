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

package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RonsenbergVI/fraise/internal/config"
)

func TestConfigSet_FromFile(t *testing.T) {
	const contents = `
[scheduler]
workers = 4
buffer-size = 128

[server]
port = 8080

[log]
level = "DEBUG"
format = "json"
disable-timestamp = true

[engine]
allow-unanchored-recall = true
half-life = "168h"
cache-capacity = 1024

[db]
precision = "float32"
default-top = 10
default-depth = 3
seed-size = 64

[db.hashing-function]
name = "xxhash"
`

	dir := t.TempDir()
	path := filepath.Join(dir, config.DefaultConfigFile)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	c := config.New()
	if _, err := c.FromFile(path); err != nil {
		t.Fatalf("FromFile returned error: %v", err)
	}

	if c.Scheduler.Workers != 4 {
		t.Errorf("Scheduler.Workers: got %d, want 4", c.Scheduler.Workers)
	}
	if c.Scheduler.BufferSize != 128 {
		t.Errorf("Scheduler.BufferSize: got %d, want 128", c.Scheduler.BufferSize)
	}
	if c.Server.Port != 8080 {
		t.Errorf("Server.Port: got %d, want 8080", c.Server.Port)
	}
	if c.Log.Level != "DEBUG" {
		t.Errorf("Log.Level: got %q, want %q", c.Log.Level, "DEBUG")
	}
	if c.Log.Format != "json" {
		t.Errorf("Log.Format: got %q, want %q", c.Log.Format, "json")
	}
	if !c.Log.DisableTimestamp {
		t.Errorf("Log.DisableTimestamp: got false, want true")
	}
	if !c.Engine.AllowUnanchoredRecall {
		t.Errorf("Engine.AllowUnanchoredRecall: got false, want true")
	}
	if c.Engine.Halflife != 168*time.Hour {
		t.Errorf("Engine.Halflife: got %v, want %v", c.Engine.Halflife, 168*time.Hour)
	}
	if c.DB.Precision != "float32" {
		t.Errorf("DB.Precision: got %q, want %q", c.DB.Precision, "float32")
	}
	if c.DB.SeedSize != 64 {
		t.Errorf("DB.SeedSize: got %d, want 64", c.DB.SeedSize)
	}
	if c.Engine.CacheCapacity != 1024 {
		t.Errorf("Engine.CacheCapacity: got %d, want 1024", c.Engine.CacheCapacity)
	}
	if c.DB.DefaultTop != 10 {
		t.Errorf("DB.DefaultTop: got %d, want 10", c.DB.DefaultTop)
	}
	if c.DB.DefaultDepth != 3 {
		t.Errorf("DB.DefaultDepth: got %d, want 3", c.DB.DefaultDepth)
	}
	if c.DB.HashingFunction.Name != "xxhash" {
		t.Errorf("DB.HashingFunction.Name: got %q, want %q", c.DB.HashingFunction.Name, "xxhash")
	}
}

func TestConfigSet_FromFile_Missing(t *testing.T) {
	c := config.New()
	if _, err := c.FromFile(filepath.Join(t.TempDir(), "does-not-exist.toml")); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// TestConfigSet_LimitsAndTimeouts checks that the robustness knobs (HTTP
// timeouts, body cap, and the top/depth/vector ceilings) decode from the config
// file, and that config.New seeds them with their documented defaults.
func TestConfigSet_LimitsAndTimeouts(t *testing.T) {
	// Defaults come from config.New (flag defaults are applied on definition).
	c := config.New()
	if c.Server.ReadTimeout != config.DefaultReadTimeout {
		t.Errorf("Server.ReadTimeout default: got %v, want %v", c.Server.ReadTimeout, config.DefaultReadTimeout)
	}
	if c.Server.MaxBodyBytes != config.DefaultMaxBodyBytes {
		t.Errorf("Server.MaxBodyBytes default: got %d, want %d", c.Server.MaxBodyBytes, config.DefaultMaxBodyBytes)
	}
	if c.Scheduler.EnqueueTimeout != config.DefaultEnqueueTimeout {
		t.Errorf("Scheduler.EnqueueTimeout default: got %v, want %v", c.Scheduler.EnqueueTimeout, config.DefaultEnqueueTimeout)
	}
	if c.DB.MaxTop != config.DefaultMaxTop {
		t.Errorf("DB.MaxTop default: got %d, want %d", c.DB.MaxTop, config.DefaultMaxTop)
	}
	if c.DB.MaxDepth != config.DefaultMaxDepth {
		t.Errorf("DB.MaxDepth default: got %d, want %d", c.DB.MaxDepth, config.DefaultMaxDepth)
	}
	if c.DB.MaxVectorDimension != config.DefaultMaxVectorDimension {
		t.Errorf("DB.MaxVectorDimension default: got %d, want %d", c.DB.MaxVectorDimension, config.DefaultMaxVectorDimension)
	}
	if c.DB.RRFK != config.DefaultRRFK {
		t.Errorf("DB.RRFK default: got %d, want %d", c.DB.RRFK, config.DefaultRRFK)
	}

	const contents = `
[server]
read-timeout = "5s"
read-header-timeout = "2s"
write-timeout = "7s"
idle-timeout = "30s"
shutdown-grace = "3s"
max-body-bytes = 4096

[db]
max-top = 50
max-depth = 4
max-vector-dimension = 128
rrf-k = 90
`
	dir := t.TempDir()
	path := filepath.Join(dir, config.DefaultConfigFile)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	f := config.New()
	if _, err := f.FromFile(path); err != nil {
		t.Fatalf("FromFile returned error: %v", err)
	}

	if f.Server.ReadTimeout != 5*time.Second {
		t.Errorf("Server.ReadTimeout: got %v, want 5s", f.Server.ReadTimeout)
	}
	if f.Server.ReadHeaderTimeout != 2*time.Second {
		t.Errorf("Server.ReadHeaderTimeout: got %v, want 2s", f.Server.ReadHeaderTimeout)
	}
	if f.Server.WriteTimeout != 7*time.Second {
		t.Errorf("Server.WriteTimeout: got %v, want 7s", f.Server.WriteTimeout)
	}
	if f.Server.IdleTimeout != 30*time.Second {
		t.Errorf("Server.IdleTimeout: got %v, want 30s", f.Server.IdleTimeout)
	}
	if f.Server.ShutdownGrace != 3*time.Second {
		t.Errorf("Server.ShutdownGrace: got %v, want 3s", f.Server.ShutdownGrace)
	}
	if f.Server.MaxBodyBytes != 4096 {
		t.Errorf("Server.MaxBodyBytes: got %d, want 4096", f.Server.MaxBodyBytes)
	}
	if f.DB.MaxTop != 50 {
		t.Errorf("DB.MaxTop: got %d, want 50", f.DB.MaxTop)
	}
	if f.DB.MaxDepth != 4 {
		t.Errorf("DB.MaxDepth: got %d, want 4", f.DB.MaxDepth)
	}
	if f.DB.MaxVectorDimension != 128 {
		t.Errorf("DB.MaxVectorDimension: got %d, want 128", f.DB.MaxVectorDimension)
	}
	if f.DB.RRFK != 90 {
		t.Errorf("DB.RRFK: got %d, want 90", f.DB.RRFK)
	}
}

// missingConfig returns a -config path that does not exist, so Parse takes the
// no-config-file branch — the one an operator running the binary with nothing
// but flags is on, and where validation used to be skipped entirely.
func missingConfig(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "does-not-exist.toml")
}

// TestParseCanonicalisesLogFlags is the bug report: `-log-level error` is what
// an operator types, and it used to be dropped on the floor — matching no arm
// of the logger's case-sensitive switch, it left the level at INFO with no
// indication that the flag had been ignored.
func TestParseCanonicalisesLogFlags(t *testing.T) {
	cases := []struct {
		args         []string
		level, forma string
	}{
		{[]string{"-log-level", "error"}, config.LogLevelError, config.LogFormatText},
		{[]string{"-log-level", "Debug"}, config.LogLevelDebug, config.LogFormatText},
		{[]string{"-log-format", "JSON"}, config.LogLevelInfo, config.LogFormatJSON},
		{[]string{"-log-level", "warn", "-log-format", "json"}, config.LogLevelWarn, config.LogFormatJSON},
	}

	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			c := config.New()
			args := append([]string{"-config", missingConfig(t)}, tc.args...)

			// The missing config file is reported, but it is not the failure
			// under test — only that it did not stop the flags being validated.
			if err := c.Parse(args); errors.Is(err, config.ErrInvalidValue) {
				t.Fatalf("Parse(%v) rejected a valid value: %v", tc.args, err)
			}
			if c.Log.Level != tc.level {
				t.Errorf("Log.Level = %q, want %q", c.Log.Level, tc.level)
			}
			if c.Log.Format != tc.forma {
				t.Errorf("Log.Format = %q, want %q", c.Log.Format, tc.forma)
			}
		})
	}
}

// TestParseRejectsUnknownLogFlags pins that an unusable value stops startup
// even with no config file to read. Parse used to return the moment the file
// was missing, so adjust and validate never ran and the value reached the
// logger unchecked — validation that only runs when a config file happens to
// exist is not validation.
func TestParseRejectsUnknownLogFlags(t *testing.T) {
	for _, args := range [][]string{
		{"-log-level", "verbose"},
		{"-log-level", "trace"},
		{"-log-format", "console"},
		{"-log-format", "logfmt"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			c := config.New()
			err := c.Parse(append([]string{"-config", missingConfig(t)}, args...))

			if !errors.Is(err, config.ErrInvalidValue) {
				t.Fatalf("Parse(%v) = %v, want an ErrInvalidValue so startup stops", args, err)
			}
			if !strings.Contains(err.Error(), "accepted:") {
				t.Errorf("error %q does not list the accepted values", err)
			}
		})
	}
}

// TestParseRejectsUnknownDBFlags extends the same guarantee to the settings
// that pick an implementation rather than a log destination, where a silent
// fallback is worse than a noisy one: -precision fell back to float64 (not even
// its own default of float32), so a typo changed the numeric precision of every
// score in the store, and a mistyped hashing function would have keyed the
// store differently than asked with nothing to show for it.
func TestParseRejectsUnknownDBFlags(t *testing.T) {
	for _, args := range [][]string{
		{"-precision", "floast32"},
		{"-precision", "float16"},
		{"-hashing-function", "murmur"},
		{"-search-algorithm", "dfs"},
		{"-ranking-algorithm", "hits"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			c := config.New()
			err := c.Parse(append([]string{"-config", missingConfig(t)}, args...))

			if !errors.Is(err, config.ErrInvalidValue) {
				t.Fatalf("Parse(%v) = %v, want an ErrInvalidValue so startup stops", args, err)
			}
			if !strings.Contains(err.Error(), "accepted:") {
				t.Errorf("error %q does not list the accepted values", err)
			}
		})
	}
}

// TestParseCanonicalisesDBFlags is the counterpart: the casing an operator
// types is accepted, and lands on the spelling the consumers switch on.
func TestParseCanonicalisesDBFlags(t *testing.T) {
	c := config.New()
	args := []string{
		"-config", missingConfig(t),
		"-precision", "Float64",
		"-hashing-function", "T1ha",
		"-search-algorithm", "BFS",
		"-ranking-algorithm", "PAGERANK",
	}

	if err := c.Parse(args); errors.Is(err, config.ErrInvalidValue) {
		t.Fatalf("Parse rejected valid values: %v", err)
	}

	if c.DB.Precision != config.PrecisionFloat64 {
		t.Errorf("DB.Precision = %q, want %q", c.DB.Precision, config.PrecisionFloat64)
	}
	if c.DB.HashingFunction.Name != config.HashingT1ha {
		t.Errorf("DB.HashingFunction.Name = %q, want %q", c.DB.HashingFunction.Name, config.HashingT1ha)
	}
	if c.DB.SearchAlgorithm.Name != config.SearchBFS {
		t.Errorf("DB.SearchAlgorithm.Name = %q, want %q", c.DB.SearchAlgorithm.Name, config.SearchBFS)
	}
	if c.DB.RankingAlgorithm.Name != config.RankingPageRank {
		t.Errorf("DB.RankingAlgorithm.Name = %q, want %q", c.DB.RankingAlgorithm.Name, config.RankingPageRank)
	}
}

// TestParseCanonicalisesConfigFileValues checks the file path gets the same
// treatment as the flags: level = "error" in TOML is as reasonable a thing to
// write as it is to type, and must land on the same canonical value.
func TestParseCanonicalisesConfigFileValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), config.DefaultConfigFile)
	contents := "[log]\nlevel = \"error\"\nformat = \"Json\"\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	c := config.New()
	if err := c.Parse([]string{"-config", path}); err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if c.Log.Level != config.LogLevelError {
		t.Errorf("Log.Level = %q, want %q", c.Log.Level, config.LogLevelError)
	}
	if c.Log.Format != config.LogFormatJSON {
		t.Errorf("Log.Format = %q, want %q", c.Log.Format, config.LogFormatJSON)
	}
}

// TestParseAppliesDefaultsWithoutAConfigFile guards the flip side of running
// validation on the no-file path: a missing config file must still be
// survivable, reported as a parse failure and not as an invalid value, with
// every default in place. The server keeps starting in that case.
func TestParseAppliesDefaultsWithoutAConfigFile(t *testing.T) {
	c := config.New()
	err := c.Parse([]string{"-config", missingConfig(t)})

	if !errors.Is(err, config.ErrParsingFailed) {
		t.Fatalf("Parse with no config file = %v, want ErrParsingFailed", err)
	}
	if errors.Is(err, config.ErrInvalidValue) {
		t.Error("a missing config file must not read as an invalid value: that stops startup")
	}
	if c.Log.Level != config.DefaultLogLevel || c.Log.Format != config.DefaultLogFormat {
		t.Errorf("log defaults not applied: level %q, format %q", c.Log.Level, c.Log.Format)
	}
	if c.Server.Port != config.DefaultPort {
		t.Errorf("Server.Port = %d, want the default %d", c.Server.Port, config.DefaultPort)
	}
}
