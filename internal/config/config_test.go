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
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/FraiseHQ/fraise/internal/config"
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

// TestConfigSet_FromFile_Missing pins that a file that is not there is told
// apart from one that is there and broken: only the first is survivable, so
// it must not read as ErrParsingFailed.
func TestConfigSet_FromFile_Missing(t *testing.T) {
	c := config.New()
	_, err := c.FromFile(filepath.Join(t.TempDir(), "does-not-exist.toml"))
	if !errors.Is(err, config.ErrMissingFile) {
		t.Fatalf("FromFile(missing) = %v, want ErrMissingFile", err)
	}
	if errors.Is(err, config.ErrParsingFailed) {
		t.Errorf("a missing file read as ErrParsingFailed, which stops startup: %v", err)
	}
}

// TestParseRejectsAnUnusableConfigFile pins the other side: a file that
// exists but cannot be used stops Parse with ErrParsingFailed, and the error
// names the file and what was wrong, so the operator can find the line.
// Starting anyway would run on defaults the operator believes they overrode.
// The unknown key is a removed setting, the kind of line an old config file
// still carries.
func TestParseRejectsAnUnusableConfigFile(t *testing.T) {
	cases := []struct {
		name, contents, detail string
	}{
		{"not TOML", "[log\nlevel = \"DEBUG\"\n", "toml"},
		{"wrong type", "[server]\nport = \"9876\"\n", "port"},
		{"unknown key", "[engine]\nallow-unanchored-recall = true\n", "engine.allow-unanchored-recall"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), config.DefaultConfigFile)
			if err := os.WriteFile(path, []byte(tc.contents), 0o644); err != nil {
				t.Fatalf("writing config file: %v", err)
			}

			err := config.New().Parse([]string{"-config", path})

			if !errors.Is(err, config.ErrParsingFailed) {
				t.Fatalf("Parse = %v, want ErrParsingFailed", err)
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("error %q does not name the file %q", err, path)
			}
			if !strings.Contains(err.Error(), tc.detail) {
				t.Errorf("error %q does not say what was wrong (%q)", err, tc.detail)
			}
		})
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
}

// TestConfigSet_ScoreCutoff pins the score cutoff's resting state and its
// decoding. Off is the contract: a config that never mentions it must leave
// every recall filling to top.
func TestConfigSet_ScoreCutoff(t *testing.T) {
	c := config.New()
	if c.DB.MinScoreRatio != 0 {
		t.Errorf("DB.MinScoreRatio default: got %v, want 0 (off)", c.DB.MinScoreRatio)
	}

	const contents = `
[db]
min-score-ratio = 0.3
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
	if f.DB.MinScoreRatio != 0.3 {
		t.Errorf("DB.MinScoreRatio: got %v, want 0.3", f.DB.MinScoreRatio)
	}

	// Parse with no file and no flags is the operator who never heard of the
	// knob: adjust must leave the cutoff off.
	p := config.New()
	if err := p.Parse([]string{"-config", missingConfig(t)}); !errors.Is(err, config.ErrMissingFile) {
		t.Fatalf("Parse() error = %v, want ErrMissingFile", err)
	}
	if p.DB.MinScoreRatio != 0 {
		t.Errorf("DB.MinScoreRatio after Parse: got %v, want 0 (off)", p.DB.MinScoreRatio)
	}

	// The flag reaches the same field.
	fl := config.New()
	if err := fl.Parse([]string{"-config", missingConfig(t), "-min-score-ratio", "0.1"}); !errors.Is(err, config.ErrMissingFile) {
		t.Fatalf("Parse() error = %v, want ErrMissingFile", err)
	}
	if fl.DB.MinScoreRatio != 0.1 {
		t.Errorf("DB.MinScoreRatio from flag: got %v, want 0.1", fl.DB.MinScoreRatio)
	}
}

// TestConfigSet_WindowAndAggregate pins the resting state of the two
// retrieval settings added for the window and the aggregation: the window on
// and the slots spread, at the recall setting that was benchmarked, so a
// config that never mentions them runs the engine that was measured; group
// hits are opt-in; a negative share and the name none are the two ways off,
// and 0 for the share means the default, as it does for the half-life. The
// file and the flags reach the same fields.
func TestConfigSet_WindowAndAggregate(t *testing.T) {
	c := config.New()
	if c.DB.WindowGamma != 0.3 {
		t.Errorf("DB.WindowGamma default: got %v, want 0.3", c.DB.WindowGamma)
	}
	want := config.Aggregate{Name: config.AggregateSpread, Pool: 60, Spread: 2, Ratio: 0.5, Cap: 1, MinSize: 3, MaxGroups: 2}
	if c.DB.Aggregate != want {
		t.Errorf("DB.Aggregate default: got %+v, want %+v", c.DB.Aggregate, want)
	}

	off := config.New()
	if err := off.Parse([]string{"-config", missingConfig(t), "-window-gamma", "-1", "-aggregate", "none"}); !errors.Is(err, config.ErrMissingFile) {
		t.Fatalf("Parse() error = %v, want ErrMissingFile", err)
	}
	if off.DB.WindowGamma != -1 || off.DB.Aggregate.Name != config.AggregateNone {
		t.Errorf("off: got gamma %v, aggregate %q, want -1 and none to survive Parse", off.DB.WindowGamma, off.DB.Aggregate.Name)
	}
	zero := config.New()
	if err := zero.Parse([]string{"-config", missingConfig(t), "-window-gamma", "0"}); !errors.Is(err, config.ErrMissingFile) {
		t.Fatalf("Parse() error = %v, want ErrMissingFile", err)
	}
	if zero.DB.WindowGamma != 0.3 {
		t.Errorf("DB.WindowGamma from an explicit 0: got %v, want the default 0.3", zero.DB.WindowGamma)
	}

	const contents = `
[db]
window-gamma = 0.5

[db.aggregate]
name = "spread"
pool = 40
`
	dir := t.TempDir()
	path := filepath.Join(dir, config.DefaultConfigFile)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing config file: %v", err)
	}
	f := config.New()
	if err := f.Parse([]string{"-config", path}); err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if f.DB.WindowGamma != 0.5 {
		t.Errorf("DB.WindowGamma: got %v, want 0.5", f.DB.WindowGamma)
	}
	want = config.Aggregate{Name: config.AggregateSpread, Pool: 40, Spread: 2, Ratio: 0.5, Cap: 1, MinSize: 3, MaxGroups: 2}
	if f.DB.Aggregate != want {
		t.Errorf("DB.Aggregate from file: got %+v, want %+v (the unset thresholds keep their defaults)", f.DB.Aggregate, want)
	}

	fl := config.New()
	if err := fl.Parse([]string{"-config", missingConfig(t), "-window-gamma", "0.5", "-aggregate", "GROUP", "-aggregate-cap", "2", "-aggregate-max-groups", "1"}); !errors.Is(err, config.ErrMissingFile) {
		t.Fatalf("Parse() error = %v, want ErrMissingFile", err)
	}
	if fl.DB.WindowGamma != 0.5 || fl.DB.Aggregate.Name != config.AggregateGroup || fl.DB.Aggregate.Cap != 2 || fl.DB.Aggregate.MaxGroups != 1 {
		t.Errorf("flags: got gamma %v, aggregate %+v, want 0.5 and group with cap 2 and max-groups 1", fl.DB.WindowGamma, fl.DB.Aggregate)
	}
}

// missingConfig returns a -config path that does not exist, so Parse takes the
// no-config-file branch, the one an operator running the binary with nothing
// but flags is on.
func missingConfig(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "does-not-exist.toml")
}

// TestParseCanonicalisesLogFlags pins that the log flags accept any casing and
// land on the canonical spelling: `-log-level error` is what an operator
// types, and it must set the level to ERROR rather than be ignored.
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

// TestParseKeepsTimestampsOnByDefault pins log.disable-timestamp's default:
// false, so timestamps stay on unless asked off. A log read from a file or a
// terminal has no other clock.
func TestParseKeepsTimestampsOnByDefault(t *testing.T) {
	c := config.New()
	if err := c.Parse([]string{"-config", missingConfig(t)}); errors.Is(err, config.ErrInvalidValue) {
		t.Fatalf("Parse with no config file rejected a default: %v", err)
	}
	if c.Log.DisableTimestamp {
		t.Error("Log.DisableTimestamp = true, want the default false: timestamps stay on unless asked off")
	}
}

// TestParseRejectsUnknownLogFlags pins that an unusable value stops startup
// even with no config file to read: validation that ran only when a config
// file happened to exist would let a flag's value reach the logger unchecked.
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
// fallback is worse than a noisy one: a mistyped precision could hold every
// score at a precision the operator did not ask for, and a mistyped hashing
// function would key the store differently than asked, with nothing to show
// for it.
func TestParseRejectsUnknownDBFlags(t *testing.T) {
	for _, args := range [][]string{
		{"-precision", "floast32"},
		{"-precision", "float16"},
		{"-hashing-function", "murmur"},
		{"-search-algorithm", "dfs"},
		{"-ranking-algorithm", "hits"},
		{"-scoring-algorithm", "tfidf"},
		{"-relevance-model", "tfidf"},
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
		"-search-algorithm", "EXCESS",
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
	if c.DB.SearchAlgorithm.Name != config.SearchExcess {
		t.Errorf("DB.SearchAlgorithm.Name = %q, want %q", c.DB.SearchAlgorithm.Name, config.SearchExcess)
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
// survivable, reported as ErrMissingFile and not as a parse failure or an
// invalid value, with every default in place. The server keeps starting in
// that case.
func TestParseAppliesDefaultsWithoutAConfigFile(t *testing.T) {
	c := config.New()
	err := c.Parse([]string{"-config", missingConfig(t)})

	if !errors.Is(err, config.ErrMissingFile) {
		t.Fatalf("Parse with no config file = %v, want ErrMissingFile", err)
	}
	if errors.Is(err, config.ErrParsingFailed) || errors.Is(err, config.ErrInvalidValue) {
		t.Error("a missing config file must not read as a parse failure or an invalid value: both stop startup")
	}
	if c.Log.Level != config.DefaultLogLevel || c.Log.Format != config.DefaultLogFormat {
		t.Errorf("log defaults not applied: level %q, format %q", c.Log.Level, c.Log.Format)
	}
	if c.Server.Port != config.DefaultPort {
		t.Errorf("Server.Port = %d, want the default %d", c.Server.Port, config.DefaultPort)
	}

	wantWorkers := max(config.MinWorkersCount, runtime.GOMAXPROCS(0))
	if c.Scheduler.Workers != wantWorkers {
		t.Errorf("Scheduler.Workers = %d, want max(MinWorkersCount, GOMAXPROCS) = %d", c.Scheduler.Workers, wantWorkers)
	}
}

// TestParseDerivesTheMCPAddressFromThePort pins the bridge's default: with no
// address configured it forwards to the daemon the same config describes, so
// a port moved in the shared file moves the bridge with it — while an
// explicit -addr wins over the derivation.
func TestParseDerivesTheMCPAddressFromThePort(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"-port", "4242"}, "http://127.0.0.1:4242"},
		{nil, "http://127.0.0.1:9876"},
		{[]string{"-port", "4242", "-addr", "http://10.0.0.5:9876"}, "http://10.0.0.5:9876"},
	}

	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			c := config.New()
			if err := c.Parse(append([]string{"-config", missingConfig(t)}, tc.args...)); errors.Is(err, config.ErrInvalidValue) || errors.Is(err, config.ErrInvalidFlag) {
				t.Fatalf("Parse(%v) = %v, want the flags accepted", tc.args, err)
			}
			if c.MCP.Address != tc.want {
				t.Errorf("MCP.Address = %q, want %q", c.MCP.Address, tc.want)
			}
		})
	}
}
