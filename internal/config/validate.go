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
	"strings"
)

// The blocks below are the canonical spellings each setting with a fixed
// vocabulary accepts, one block per setting. They are a vocabulary, not
// defaults: they say what a setting may name, while constants.go says which of
// them applies when nothing is configured. A value is matched against them
// case-insensitively and rewritten to the spelling here, so everything
// downstream compares against one form and an operator can write "error" or
// "ERROR" without the difference deciding anything.

// log.level — the minimum severity emitted.
const (
	LogLevelDebug string = "DEBUG"
	LogLevelInfo  string = "INFO"
	LogLevelWarn  string = "WARN"
	LogLevelError string = "ERROR"
)

// log.format — the encoding each line is written in.
const (
	LogFormatText string = "text"
	LogFormatJSON string = "json"
)

// db.precision — the float width embeddings and scores are held at, and so
// which generic instantiation of the server is built.
const (
	PrecisionFloat32 string = "float32"
	PrecisionFloat64 string = "float64"
)

// db.hashing-function.name — the hash that derives node keys from values.
const (
	HashingXxhash string = "xxhash"
	HashingT1ha   string = "t1ha"
)

// db.search-algorithm.name — the traversal moving seed evidence through the
// graph; "none" turns the graph channel off (text/vector search only).
const (
	SearchNone   string = "none"
	SearchBFS    string = "bfs"
	SearchExcess string = "excess"
)

// db.scoring-algorithm.name — the fold deriving relevance from pooled
// contributions.
const (
	ScoringExcess string = "excess"
	ScoringRRF    string = "rrf"
)

// db.relevance-model.name — the text index's relevance model.
const (
	RelevanceBM25       string = "bm25"
	RelevanceMatchCount string = "matchcount"
)

// db.ranking-algorithm.name — the global boost applied to relevance scores;
// "none" disables it.
const (
	RankingNone     string = "none"
	RankingPageRank string = "pagerank"
)

// The values each setting accepts, in the order the error message lists them.
// They are the single source of truth for the accepted set, and consumers
// switch on the same constants. A value added here needs a case in each of
// them: a missing case still compiles, and the value silently gets that
// consumer's fallback.
var (
	LogLevels = []string{LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError}

	LogFormats = []string{LogFormatText, LogFormatJSON}

	Precisions = []string{PrecisionFloat32, PrecisionFloat64}

	HashingFunctions = []string{HashingXxhash, HashingT1ha}

	SearchAlgorithms = []string{SearchNone, SearchBFS, SearchExcess}

	ScoringAlgorithms = []string{ScoringExcess, ScoringRRF}

	RelevanceModels = []string{RelevanceBM25, RelevanceMatchCount}

	RankingAlgorithms = []string{RankingNone, RankingPageRank}
)

// Canonical rewrites *v to whichever accepted value it matches
// case-insensitively. If none does, it leaves *v as typed and returns
// ErrInvalidValue listing the accepted values.
//
// Matching case-insensitively accepts whatever casing an operator types, such
// as `-log-level error`; rewriting to the canonical spelling lets every
// consumer downstream compare with ==. name is the setting's dotted config
// path, so the message points at the line to edit rather than at a bare value.
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
// Every setting with a fixed vocabulary belongs here: one left out gets its
// consumer's silent fallback, and nothing makes that visible from the
// outside. The same holds for a numeric setting with a bounded domain:
// db.min-score-ratio is a fraction of the best hit's relevance, and a value
// past 1 (an operator thinking in percent) would put the bar above every hit
// and silently empty every recall.
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
		{&c.DB.RankingAlgorithm.Name, "db.ranking-algorithm.name", RankingAlgorithms},
		{&c.DB.ScoringAlgorithm.Name, "db.scoring-algorithm.name", ScoringAlgorithms},
		{&c.DB.RelevanceModel.Name, "db.relevance-model.name", RelevanceModels},
	}

	for _, s := range settings {
		if err := Canonical(s.value, s.name, s.accepted); err != nil {
			return err
		}
	}

	// Written as the accepted range rather than its complement so a NaN, which
	// fails every comparison, is rejected too instead of slipping through.
	if r := c.DB.MinScoreRatio; !(r >= 0 && r <= 1) {
		return fmt.Errorf("%w: db.min-score-ratio = %v (accepted: 0 to 1)", ErrInvalidValue, r)
	}
	return nil
}
