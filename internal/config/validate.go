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
	"fmt"
	"math"
	"strings"
)

// The blocks below hold the canonical spellings each fixed-vocabulary setting
// accepts, one block per setting. They are a vocabulary, not defaults. They
// say what a setting may name, and constants.go says which one applies when
// nothing is configured. A value is matched against them case-insensitively
// and rewritten to the spelling here. So everything downstream compares
// against one form, and an operator can write "error" or "ERROR" alike.

// log.level sets the minimum severity emitted.
const (
	LogLevelDebug string = "DEBUG"
	LogLevelInfo  string = "INFO"
	LogLevelWarn  string = "WARN"
	LogLevelError string = "ERROR"
)

// log.format sets the encoding each line is written in.
const (
	LogFormatText string = "text"
	LogFormatJSON string = "json"
)

// db.precision sets the float width of embeddings and scores. It also picks
// which generic instantiation of the server is built.
const (
	PrecisionFloat32 string = "float32"
	PrecisionFloat64 string = "float64"
)

// db.hashing-function.name names the hash that derives node keys from values.
const (
	HashingXxhash string = "xxhash"
	HashingT1ha   string = "t1ha"
)

// db.search-algorithm.name names the traversal that moves seed evidence
// through the graph. "none" turns the graph channel off, leaving text and
// vector search only.
const (
	SearchNone   string = "none"
	SearchBFS    string = "bfs"
	SearchExcess string = "excess"
)

// db.scoring-algorithm.name names the fold that derives relevance from pooled
// contributions.
const (
	ScoringInformation string = "information"
	ScoringExcess      string = "excess"
	ScoringRRF         string = "rrf"
)

// db.relevance-model.name names the text index's relevance model.
const (
	RelevanceBM25       string = "bm25"
	RelevanceMatchCount string = "matchcount"
)

// The values each setting accepts, in the order the error message lists them.
// They are the single source of truth for the accepted set. Consumers switch
// on the same constants, so a value added here needs a case in each consumer.
// A missing case still compiles, and the value silently gets that consumer's
// fallback.
var (
	LogLevels = []string{LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError}

	LogFormats = []string{LogFormatText, LogFormatJSON}

	Precisions = []string{PrecisionFloat32, PrecisionFloat64}

	HashingFunctions = []string{HashingXxhash, HashingT1ha}

	SearchAlgorithms = []string{SearchNone, SearchBFS, SearchExcess}

	ScoringAlgorithms = []string{ScoringInformation, ScoringExcess, ScoringRRF}

	RelevanceModels = []string{RelevanceBM25, RelevanceMatchCount}
)

// Canonical rewrites *v to whichever accepted value it matches
// case-insensitively. If none does, it leaves *v as typed and returns
// ErrInvalidValue listing the accepted values.
//
// The match accepts any casing an operator types, such as '-log-level error'.
// The rewrite lets every consumer downstream compare with ==. name is the
// setting's dotted config path, so the message points at the line to edit
// rather than at a bare value.
func Canonical(v *string, name string, accepted []string) error {
	for _, want := range accepted {
		if strings.EqualFold(*v, want) {
			*v = want
			return nil
		}
	}
	return fmt.Errorf("%w: %s = %q (accepted: %s)", ErrInvalidValue, name, *v, strings.Join(accepted, ", "))
}

// validate rejects settings whose value the server cannot honour, so startup
// fails instead of the server quietly doing something else.
//
// Every setting with a fixed vocabulary belongs here. A setting left out
// falls back silently in its consumer, and nothing shows it from outside.
// Numeric settings with a bounded domain belong here too:
//   - db.max-depth caps a recall's depth clause. Search has no lane past 2.
//     A higher ceiling would let depth:3 through, to be answered as depth 2.
//     A negative one would reject every recall that names a depth.
//   - db.max-top must be at least 1. A lower ceiling would reject every
//     recall that names a top. It has no upper bound, because how many hits
//     one recall may ask for is the operator's call.
//   - scheduler.workers must be at least 1. Adjust only replaces a zero, so a
//     negative count would reach the scheduler. The scheduler would start no
//     worker, and every accepted query would wait forever.
//   - db.num-graphs is bounded by the selector. A selector is a uint8, so a
//     graph past 256 is allocated but no query can ever address it.
func (c *ConfigSet) validate() error {
	settings := []struct {
		value    *string
		name     string // dotted config path, so the message names the line to edit
		accepted []string
	}{
		{&c.Log.Level, "log.level", LogLevels},
		{&c.Log.Format, "log.format", LogFormats},
		{&c.DB.Precision, "db.precision", Precisions},
		{&c.DB.HashingFunction.Name, "db.hashing-function.name", HashingFunctions},
		{&c.DB.SearchAlgorithm.Name, "db.search-algorithm.name", SearchAlgorithms},
		{&c.DB.ScoringAlgorithm.Name, "db.scoring-algorithm.name", ScoringAlgorithms},
		{&c.DB.RelevanceModel.Name, "db.relevance-model.name", RelevanceModels},
	}

	for _, s := range settings {
		if err := Canonical(s.value, s.name, s.accepted); err != nil {
			return err
		}
	}

	// The pool and the cap must each be at least 1. Below that, no slot could
	// be drawn, or no fact taken for a facet.
	if p := c.DB.Aggregate.Pool; p < 1 {
		return fmt.Errorf("%w: db.aggregate.pool = %d (accepted: 1 or more)", ErrInvalidValue, p)
	}
	if n := c.DB.Aggregate.Cap; n < 1 {
		return fmt.Errorf("%w: db.aggregate.cap = %d (accepted: 1 or more)", ErrInvalidValue, n)
	}

	if w := c.Scheduler.Workers; w < 1 {
		return fmt.Errorf("%w: scheduler.workers = %d (accepted: 1 or more)", ErrInvalidValue, w)
	}

	// An operator may lower the ceiling, never raise it past the last lane.
	if d := c.DB.MaxDepth; d < 0 || d > 2 {
		return fmt.Errorf("%w: db.max-depth = %d (accepted: 0 to 2)", ErrInvalidValue, d)
	}

	if m := c.DB.MaxTop; m < 1 {
		return fmt.Errorf("%w: db.max-top = %d (accepted: 1 or more)", ErrInvalidValue, m)
	}

	if n := c.DB.NumGraphs; n < 1 || n > math.MaxUint8+1 {
		return fmt.Errorf("%w: db.num-graphs = %d (accepted: 1 to %d)", ErrInvalidValue, n, math.MaxUint8+1)
	}
	return nil
}
