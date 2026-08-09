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
	"strings"
	"testing"
)

// TestCanonicalAcceptsAnyCasing pins the half of the contract an operator
// notices: whatever casing they type is accepted and rewritten to the one
// spelling everything downstream compares against. "error" is the case from the
// bug report — it matched no arm of the logger's case-sensitive switch and so
// silently produced INFO logs.
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
// fallback. The message is asserted in full because it is the entire remedy an
// operator gets — an error that says only "invalid value" leaves them guessing
// at the spelling, which is barely better than the default it replaced.
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
// an otherwise-default config, so a value left out of validate shows up as this
// test passing where it should fail — the whole point being that no setting
// keeps a silent fallback while its neighbours are checked.
//
// The setting's dotted name is asserted too: with six of them going through one
// loop, "invalid value" alone would not tell an operator which line to fix.
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

// TestValidateCanonicalisesEverySetting pins the rewrite half for all six: a
// value typed in any casing lands on the canonical spelling, which is what the
// consumers' switches compare against.
func TestValidateCanonicalisesEverySetting(t *testing.T) {
	c := New()
	c.Log.Level = "error"
	c.Log.Format = "JSON"
	c.DB.Precision = "Float64"
	c.DB.HashingFunction.Name = "T1HA"
	c.DB.SearchAlgorithm.Name = "BFS"
	c.DB.RankingAlgorithm.Name = "PageRank"

	if err := c.validate(); err != nil {
		t.Fatalf("validate() returned error: %v", err)
	}

	for _, got := range []struct{ name, have, want string }{
		{"log.level", c.Log.Level, LogLevelError},
		{"log.format", c.Log.Format, LogFormatJSON},
		{"db.precision", c.DB.Precision, PrecisionFloat64},
		{"db.hashing-function.name", c.DB.HashingFunction.Name, HashingT1ha},
		{"db.search-algorithm.name", c.DB.SearchAlgorithm.Name, SearchBFS},
		{"db.ranking-algorithm.name", c.DB.RankingAlgorithm.Name, RankingPageRank},
	} {
		if got.have != got.want {
			t.Errorf("%s = %q, want the canonical %q", got.name, got.have, got.want)
		}
	}
}
