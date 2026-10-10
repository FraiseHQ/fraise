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

package config

import (
	"errors"
	"math"
	"strings"
	"testing"
)

// TestCanonicalAcceptsAnyCasing pins the half of the contract an operator
// notices: whatever casing they type is accepted and rewritten to the one
// spelling everything downstream compares against.
func TestCanonicalAcceptsAnyCasing(t *testing.T) {
	cases := []struct {
		value    string
		accepted []string
		want     string
	}{
		{"error", LogLevels, LogLevelError},
		{"ERROR", LogLevels, LogLevelError},
		{"Error", LogLevels, LogLevelError},
		{"debug", LogLevels, LogLevelDebug},
		{"WARN", LogLevels, LogLevelWarn},
		{"json", LogFormats, LogFormatJSON},
		{"JSON", LogFormats, LogFormatJSON},
		{"Text", LogFormats, LogFormatText},
	}

	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			got := tc.value
			if err := Canonical(&got, "log.level", tc.accepted); err != nil {
				t.Fatalf("Canonical(%q) returned error: %v", tc.value, err)
			}
			if got != tc.want {
				t.Errorf("Canonical(%q) = %q, want the canonical %q", tc.value, got, tc.want)
			}
		})
	}
}

// TestCanonicalRejectsUnknownValue pins the other half: an unrecognised value
// is an error naming the setting and listing what it accepts, not a silent
// fallback, and the value is left as typed. Each part of the message is
// asserted because the message is the operator's only remedy: an error that
// says only "invalid value" leaves them guessing at the spelling.
func TestCanonicalRejectsUnknownValue(t *testing.T) {
	value := "verbose"
	err := Canonical(&value, "log.level", LogLevels)

	if err == nil {
		t.Fatal("Canonical(\"verbose\") = nil error, want a rejection")
	}
	if !errors.Is(err, ErrInvalidValue) {
		t.Errorf("error %v is not an ErrInvalidValue, so callers cannot tell it from a missing config file", err)
	}
	for _, want := range []string{"log.level", `"verbose"`, "DEBUG, INFO, WARN, ERROR"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if value != "verbose" {
		t.Errorf("value was rewritten to %q on failure, want it left as typed", value)
	}
}

// TestValidateAcceptsTheDefaults is the floor: the configuration the server
// runs with when nothing is set must itself be valid. A default that its own
// validator rejects would make the binary refuse to start out of the box.
func TestValidateAcceptsTheDefaults(t *testing.T) {
	c := New()
	if err := c.validate(); err != nil {
		t.Fatalf("validate() on the defaults returned error: %v", err)
	}
}

// TestValidateChecksEverySetting guards against a setting being validated in
// one place and forgotten in another. Each case poisons exactly one setting of
// an otherwise-default config, so a setting left out of validate fails its
// case: no setting may keep a silent fallback while its neighbours are
// checked.
//
// The setting's dotted name is asserted too: with this many settings checked in
// one function, "invalid value" alone would not tell an operator which line to
// fix.
func TestValidateChecksEverySetting(t *testing.T) {
	cases := []struct {
		name   string // the dotted path the error must name
		poison func(*ConfigSet, string)
	}{
		{"log.level", func(c *ConfigSet, v string) { c.Log.Level = v }},
		{"log.format", func(c *ConfigSet, v string) { c.Log.Format = v }},
		{"db.precision", func(c *ConfigSet, v string) { c.DB.Precision = v }},
		{"db.hashing-function.name", func(c *ConfigSet, v string) { c.DB.HashingFunction.Name = v }},
		{"db.search-algorithm.name", func(c *ConfigSet, v string) { c.DB.SearchAlgorithm.Name = v }},
		{"db.ranking-algorithm.name", func(c *ConfigSet, v string) { c.DB.RankingAlgorithm.Name = v }},
		{"db.scoring-algorithm.name", func(c *ConfigSet, v string) { c.DB.ScoringAlgorithm.Name = v }},
		{"db.relevance-model.name", func(c *ConfigSet, v string) { c.DB.RelevanceModel.Name = v }},
		{"db.min-score-ratio", func(c *ConfigSet, _ string) { c.DB.MinScoreRatio = 30 }},
		{"db.window-gamma", func(c *ConfigSet, _ string) { c.DB.WindowGamma = 1.5 }},
		{"db.aggregate.name", func(c *ConfigSet, v string) { c.DB.Aggregate.Name = v }},
		{"db.aggregate.ratio", func(c *ConfigSet, _ string) { c.DB.Aggregate.Ratio = 2 }},
		{"db.aggregate.pool", func(c *ConfigSet, _ string) { c.DB.Aggregate.Pool = 0 }},
		{"db.aggregate.spread", func(c *ConfigSet, _ string) { c.DB.Aggregate.Spread = 0 }},
		{"db.aggregate.min-size", func(c *ConfigSet, _ string) { c.DB.Aggregate.MinSize = 1 }},
		{"db.aggregate.max-groups", func(c *ConfigSet, _ string) { c.DB.Aggregate.MaxGroups = 0 }},
		{"scheduler.workers", func(c *ConfigSet, _ string) { c.Scheduler.Workers = -1 }},
		{"db.max-depth", func(c *ConfigSet, _ string) { c.DB.MaxDepth = 3 }},
		{"db.max-top", func(c *ConfigSet, _ string) { c.DB.MaxTop = -1 }},
		{"db.num-graphs", func(c *ConfigSet, _ string) { c.DB.NumGraphs = 300 }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := New()
			tc.poison(c, "nonsense")

			err := c.validate()
			if !errors.Is(err, ErrInvalidValue) {
				t.Fatalf("validate() with a bad %s = %v, want an ErrInvalidValue", tc.name, err)
			}
			if !strings.Contains(err.Error(), tc.name) {
				t.Errorf("error %q does not name the setting %q", err, tc.name)
			}
		})
	}
}

// TestValidateBoundsMinScoreRatio pins the domain of db.min-score-ratio: a
// fraction of the best hit's relevance, so anything outside [0, 1] is rejected
// at startup rather than acted on. Each value past the range would otherwise
// fail silently: 30 (an operator thinking in percent) puts the bar above
// every hit so every recall comes back empty, a negative ratio reads as
// "off", and NaN fails every comparison in the cutoff and empties every
// recall too. The endpoints are kept: 0 is off and 1 keeps only hits tied
// with the best, both meaningful settings.
func TestValidateBoundsMinScoreRatio(t *testing.T) {
	for _, ratio := range []float64{0, 0.3, 1} {
		c := New()
		c.DB.MinScoreRatio = ratio
		if err := c.validate(); err != nil {
			t.Errorf("validate() with min-score-ratio = %v returned error: %v, want it accepted", ratio, err)
		}
	}

	for _, ratio := range []float64{-0.1, 1.0000001, 30, math.NaN(), math.Inf(1)} {
		c := New()
		c.DB.MinScoreRatio = ratio
		err := c.validate()
		if !errors.Is(err, ErrInvalidValue) {
			t.Fatalf("validate() with min-score-ratio = %v = %v, want an ErrInvalidValue", ratio, err)
		}
		for _, want := range []string{"db.min-score-ratio", "accepted: 0 to 1"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not mention %q", err, want)
			}
		}
	}
}

// TestValidateBoundsWorkers pins the floor of scheduler.workers. Adjust only
// replaces a zero with the default, so a negative count would otherwise reach
// Scheduler.Start, whose loop starts no worker: the server would accept
// queries into its queue and never answer them, then 429 once the queue
// filled. validate runs after Adjust, so a zero has already become the default
// by then and the check never sees one.
func TestValidateBoundsWorkers(t *testing.T) {
	for _, workers := range []int{1, 4} {
		c := New()
		c.Scheduler.Workers = workers
		if err := c.validate(); err != nil {
			t.Errorf("validate() with workers = %d returned error: %v, want it accepted", workers, err)
		}
	}

	for _, workers := range []int{0, -1, math.MinInt} {
		c := New()
		c.Scheduler.Workers = workers
		err := c.validate()
		if !errors.Is(err, ErrInvalidValue) {
			t.Fatalf("validate() with workers = %d = %v, want an ErrInvalidValue", workers, err)
		}
		for _, want := range []string{"scheduler.workers", "accepted: 1 or more"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not mention %q", err, want)
			}
		}
	}
}

// TestValidateBoundsMaxDepth pins the domain of db.max-depth: the lanes are
// depth 0, 1 and 2, and search has nothing past 2. A ceiling of 3 or more
// would let depth:3 through the parser's range check to be answered exactly
// as depth 2, the silent substitution the ceiling exists to refuse; a
// negative ceiling would reject every recall that names a depth. Lowering
// the ceiling stays allowed: 1 keeps recalls off the max-recall lane.
func TestValidateBoundsMaxDepth(t *testing.T) {
	for _, depth := range []int{0, 1, 2} {
		c := New()
		c.DB.MaxDepth = depth
		if err := c.validate(); err != nil {
			t.Errorf("validate() with max-depth = %d returned error: %v, want it accepted", depth, err)
		}
	}

	for _, depth := range []int{-1, 3, 10} {
		c := New()
		c.DB.MaxDepth = depth
		err := c.validate()
		if !errors.Is(err, ErrInvalidValue) {
			t.Fatalf("validate() with max-depth = %d = %v, want an ErrInvalidValue", depth, err)
		}
		for _, want := range []string{"db.max-depth", "accepted: 0 to 2"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not mention %q", err, want)
			}
		}
	}
}

// TestValidateBoundsMaxTop pins the floor of db.max-top: query.Parse rejects
// a top: past the ceiling, so a ceiling below 1 would reject every recall that
// names a top, while recalls that omit one kept working. There is no upper
// bound: a large ceiling is a resource decision the operator owns, and the
// search sizes its allocations on candidates rather than on top.
func TestValidateBoundsMaxTop(t *testing.T) {
	for _, m := range []int{1, DefaultMaxTop, 1 << 20} {
		c := New()
		c.DB.MaxTop = m
		if err := c.validate(); err != nil {
			t.Errorf("validate() with max-top = %d returned error: %v, want it accepted", m, err)
		}
	}

	for _, m := range []int{-1, 0} {
		c := New()
		c.DB.MaxTop = m
		err := c.validate()
		if !errors.Is(err, ErrInvalidValue) {
			t.Fatalf("validate() with max-top = %d = %v, want an ErrInvalidValue", m, err)
		}
		for _, want := range []string{"db.max-top", "accepted: 1 or more"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not mention %q", err, want)
			}
		}
	}
}

// TestValidateBoundsNumGraphs pins the domain of db.num-graphs to what a
// selector can address. Selectors are uint8, so 256 graphs (@0 to @255) is the
// most a query can reach: 300 would start a server holding 44 graphs no
// selector can name, and a negative count would quietly get the default. Both
// endpoints are kept: one graph is a single-tenant store, 256 is every
// selector in use.
func TestValidateBoundsNumGraphs(t *testing.T) {
	for _, n := range []int{1, DefaultNumGraph, 256} {
		c := New()
		c.DB.NumGraphs = n
		if err := c.validate(); err != nil {
			t.Errorf("validate() with num-graphs = %d returned error: %v, want it accepted", n, err)
		}
	}

	for _, n := range []int{-1, 0, 257, 300} {
		c := New()
		c.DB.NumGraphs = n
		err := c.validate()
		if !errors.Is(err, ErrInvalidValue) {
			t.Fatalf("validate() with num-graphs = %d = %v, want an ErrInvalidValue", n, err)
		}
		for _, want := range []string{"db.num-graphs", "accepted: 1 to 256"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not mention %q", err, want)
			}
		}
	}
}

// TestValidateCanonicalisesEverySetting pins the rewrite half for all eight: a
// value typed in any casing lands on the canonical spelling, which is what the
// consumers' switches compare against.
func TestValidateCanonicalisesEverySetting(t *testing.T) {
	c := New()
	c.Log.Level = "error"
	c.Log.Format = "JSON"
	c.DB.Precision = "Float64"
	c.DB.HashingFunction.Name = "T1HA"
	c.DB.SearchAlgorithm.Name = "EXCESS"
	c.DB.RankingAlgorithm.Name = "PageRank"
	c.DB.ScoringAlgorithm.Name = "RRF"
	c.DB.RelevanceModel.Name = "BM25"

	if err := c.validate(); err != nil {
		t.Fatalf("validate() returned error: %v", err)
	}

	for _, got := range []struct{ name, have, want string }{
		{"log.level", c.Log.Level, LogLevelError},
		{"log.format", c.Log.Format, LogFormatJSON},
		{"db.precision", c.DB.Precision, PrecisionFloat64},
		{"db.hashing-function.name", c.DB.HashingFunction.Name, HashingT1ha},
		{"db.search-algorithm.name", c.DB.SearchAlgorithm.Name, SearchExcess},
		{"db.ranking-algorithm.name", c.DB.RankingAlgorithm.Name, RankingPageRank},
		{"db.scoring-algorithm.name", c.DB.ScoringAlgorithm.Name, ScoringRRF},
		{"db.relevance-model.name", c.DB.RelevanceModel.Name, RelevanceBM25},
	} {
		if got.have != got.want {
			t.Errorf("%s = %q, want the canonical %q", got.name, got.have, got.want)
		}
	}
}
