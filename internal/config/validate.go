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

// db.ranking-algorithm.name — the global boost applied to walk scores; "none"
// disables it.
const (
	RankingNone     string = "none"
	RankingPageRank string = "pagerank"
)

// The values each setting accepts, in the order the error message lists them.
// They are the single source of truth for the accepted set: every consumer
// switches on these same constants, so a value added here without a case there
// fails to compile rather than falling through to a default.
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
// case-insensitively, and rejects it if none does.
//
// Both halves matter. Matching case-insensitively is why `-log-level error`
// works — it is what an operator types first, and it used to match no case in
// the logger's switch. Rewriting to the canonical spelling is what lets every
// consumer downstream compare with ==, instead of each one deciding for itself
// whether "Json" counts. name is the setting's dotted config path, so the
// message points at the line to edit rather than at a bare value.
func Canonical(v *string, name string, accepted []string) error {
	for _, want := range accepted {
		if strings.EqualFold(*v, want) {
			*v = want
			return nil
		}
	}
	return fmt.Errorf("%w: %s = %q (accepted: %s)", ErrInvalidValue, name, *v, strings.Join(accepted, ", "))
}

// validate rejects settings whose value names something the server cannot do.
//
// The alternative — the switch-with-a-default that every one of these replaced
// — answers a request the server cannot honour by quietly doing something else:
// `-log-level error` yielded INFO logs, and the operator's only clue was the
// output they were trying to suppress still being there. db.precision was worse
// still, falling back to float64 when its own default is float32, so a typo
// changed the numeric precision of every score in the store. Startup is the one
// moment where saying so costs nothing.
//
// Every setting with a fixed vocabulary belongs here. One left out is one that
// keeps its silent fallback, and nothing about its consumer's switch makes that
// visible from the outside.
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
	return nil
}
