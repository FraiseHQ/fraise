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
	"encoding/json"
	"flag"
	"fmt"
	"runtime"
	"time"

	"github.com/BurntSushi/toml"
)

// Config
type ConfigSet struct {
	*flag.FlagSet `json:"-"`

	Scheduler SchedulerConfig `toml:"scheduler"`
	Server    ServerConfig    `toml:"server"`
	Log       LogConfig       `toml:"log"`
	Engine    EngineConfig    `toml:"engine"`
	DB        DBConfig        `toml:"db"`

	configFile string
}

type SchedulerConfig struct {
	// Number of workers scheduler has to execute read and writes
	Workers int `toml:"workers"`

	// Buffer size is the scheduler channel buffer size.
	BufferSize uint `toml:"buffer-size"`

	// Maximum time a submit waits for space in a full queue before the
	// request is rejected so clients can back off.
	EnqueueTimeout time.Duration `toml:"enqueue-timeout"`
}

type ServerConfig struct {
	Port int `toml:"port"`

	// Maximum time to read an entire request (headers + body).
	ReadTimeout time.Duration `toml:"read-timeout"`

	// Maximum time to read request headers alone (Slowloris protection).
	ReadHeaderTimeout time.Duration `toml:"read-header-timeout"`

	// Maximum time to write a response.
	WriteTimeout time.Duration `toml:"write-timeout"`

	// Maximum time a kept-alive connection may sit idle between requests.
	IdleTimeout time.Duration `toml:"idle-timeout"`

	// How long a graceful shutdown waits for in-flight requests to drain.
	ShutdownGrace time.Duration `toml:"shutdown-grace"`

	// Maximum request body size, in bytes, accepted by the query endpoint.
	MaxBodyBytes int64 `toml:"max-body-bytes"`
}

type LogConfig struct {
	// LOG LEVEL: DEBUG, INFO, WARN, ERROR (default = INFO)
	Level string `toml:"level"`

	// LOG FORMAT: text or json (default = text)
	// Note: all logs are printed in console. File logging not supported (yet)
	Format string `toml:"format"`

	// Disable log timestamp
	DisableTimestamp bool `toml:"disable-timestamp"`
}

type EngineConfig struct {
	// Allow unanchored recalls: queries of type "recall since:7d"
	AllowUnanchoredRecall bool `toml:"allow-unanchored-recall"`

	// Half life for time decay (used to score facts)
	Halflife time.Duration `toml:"half-life"`

	// Query cache size
	CacheCapacity int `toml:"cache-capacity"`
}

type DBConfig struct {
	// Floating-point precision for embeddings and scores: "float32" or
	// "float64". Selects which generic instantiation of the server is built at
	// startup (see cmd/server).
	Precision string `toml:"precision"`

	// How many independent graphs the store allocates (selectors 0..n-1)
	NumGraphs int `toml:"num-graphs"`

	// default top
	DefaultTop int `toml:"default-top"`

	// default depth
	DefaultDepth int `toml:"default-depth"`

	// Ceiling on a recall's top clause (rejected past this at parse time).
	MaxTop int `toml:"max-top"`

	// Ceiling on a recall's depth clause (rejected past this at parse time).
	MaxDepth int `toml:"max-depth"`

	// Ceiling on the length of a bound vector parameter (rejected past this at
	// parse time).
	MaxVectorDimension int `toml:"max-vector-dimension"`

	// The *minimum* candidate budget pulled from each source (keywords and
	// vector). Search widens it to the requested result size — the effective
	// budget is max(seed-size, top) — so a recall asking for more results
	// than this can never be silently starved of candidates.
	SeedSize int `toml:"seed-size"`

	// database hashing function
	HashingFunction HashingFunction `toml:"hashing-function"`

	// graph search traversal algorithm
	SearchAlgorithm SearchAlgorithm `toml:"search-algorithm"`

	// graph search ranking boost (none, pagerank)
	RankingAlgorithm RankingAlgorithm `toml:"ranking-algorithm"`

	// relevance fold (excess, rrf)
	ScoringAlgorithm ScoringAlgorithm `toml:"scoring-algorithm"`

	// text-index relevance model (bm25, matchcount)
	RelevanceModel RelevanceModel `toml:"relevance-model"`

	VectorSearch VectorSearch `toml:"vector-search"`
}

type HashingFunction struct {
	// (xxhash, t1ha)
	Name string `toml:"name"`

	// hashing function seed
	Seed uint64 `toml:"seed"`
}

type SearchAlgorithm struct {
	// name (none, bfs, excess): the traversal moving seed evidence through
	// the graph; "none" turns the graph channel off (text/vector only)
	Name string `toml:"name"`
}

type ScoringAlgorithm struct {
	// name (excess, rrf): the fold deriving each candidate's relevance from
	// its pooled contributions
	Name string `toml:"name"`
}

type RelevanceModel struct {
	// name (bm25, matchcount): the text index's relevance model — how a
	// document's match against the query becomes a number
	Name string `toml:"name"`
}
type RankingAlgorithm struct {
	// only pagerank supported (if no ranking none is accepted)
	Name string `toml:"name"`

	// PageRank probability of following an edge (used when
	// ranking-algorithm is pagerank)
	PageRankDamping float64 `toml:"pagerank-damping"`

	// PageRank iteration cap
	PageRankMaxIter int `toml:"pagerank-max-iter"`

	// PageRank convergence threshold on the score delta
	PageRankTol float64 `toml:"pagerank-tol"`
}

type VectorSearch struct {
	ProjectionDimension int `toml:"projection-dimension"`

	NumberTrees int `toml:"number-trees"`

	Seed uint64 `toml:"seed"`

	// Forest garbage compaction threshold: entries per live vector before
	// the forest is rebuilt from the live set.
	FlushFactor int `toml:"flush-factor"`
}

// Instanciates new configset
func New() *ConfigSet {
	config := &ConfigSet{}

	config.FlagSet = flag.NewFlagSet("flags", flag.PanicOnError)

	flagSet := config.FlagSet

	// config file (path to the TOML config Parse reads before applying flags)
	flagSet.StringVar(&config.configFile, "config", DefaultConfigFile, "Path to the TOML config file")

	// scheduler
	defaultWorkers := max(MinWorkersCount, runtime.GOMAXPROCS(0))
	flagSet.IntVar(&config.Scheduler.Workers, "workers", defaultWorkers, "Default worker count.")
	flagSet.UintVar(&config.Scheduler.BufferSize, "buffer-size", DefaultBufferSize, "Default Buffer size")
	flagSet.DurationVar(&config.Scheduler.EnqueueTimeout, "enqueue-timeout", DefaultEnqueueTimeout, "Max wait for queue space before rejecting a query")

	// server
	flagSet.IntVar(&config.Server.Port, "port", DefaultPort, "Server port")
	flagSet.DurationVar(&config.Server.ReadTimeout, "read-timeout", DefaultReadTimeout, "Max time to read a whole request")
	flagSet.DurationVar(&config.Server.ReadHeaderTimeout, "read-header-timeout", DefaultReadHeaderTimeout, "Max time to read request headers")
	flagSet.DurationVar(&config.Server.WriteTimeout, "write-timeout", DefaultWriteTimeout, "Max time to write a response")
	flagSet.DurationVar(&config.Server.IdleTimeout, "idle-timeout", DefaultIdleTimeout, "Max idle time on a kept-alive connection")
	flagSet.DurationVar(&config.Server.ShutdownGrace, "shutdown-grace", DefaultShutdownGrace, "Grace period for in-flight requests on shutdown")
	flagSet.Int64Var(&config.Server.MaxBodyBytes, "max-body-bytes", DefaultMaxBodyBytes, "Max request body size in bytes")

	// log
	flagSet.StringVar(&config.Log.Level, "log-level", DefaultLogLevel, "Log level")
	flagSet.StringVar(&config.Log.Format, "log-format", DefaultLogFormat, "Log Format")
	flagSet.BoolVar(&config.Log.DisableTimestamp, "log-disable-timestamp", true, "Log Format")

	// engine
	flagSet.BoolVar(&config.Engine.AllowUnanchoredRecall, "allow-unanchored-recall", DefaultAllowUnanchoredRecall, "Allow unanchored recalls")
	flagSet.DurationVar(&config.Engine.Halflife, "half-life", DefaultHalflife, "Half life for time decay")
	flagSet.IntVar(&config.Engine.CacheCapacity, "cache-capacity", DefaultCacheCapacity, "Query cache size")

	// db
	flagSet.IntVar(&config.DB.NumGraphs, "num-graphs", DefaultNumGraph, "Number of independent graphs the store allocates")
	flagSet.IntVar(&config.DB.DefaultTop, "default-top", DefaultTop, "Default Top")
	flagSet.IntVar(&config.DB.DefaultDepth, "default-depth", DefaultDepth, "Default Depth")
	flagSet.IntVar(&config.DB.MaxTop, "max-top", DefaultMaxTop, "Ceiling on a recall's top clause")
	flagSet.IntVar(&config.DB.MaxDepth, "max-depth", DefaultMaxDepth, "Ceiling on a recall's depth clause")
	flagSet.IntVar(&config.DB.MaxVectorDimension, "max-vector-dimension", DefaultMaxVectorDimension, "Ceiling on a bound vector's length")
	flagSet.StringVar(&config.DB.Precision, "precision", DefaultPrecision, "Embedding/score precision: float32 or float64")
	flagSet.IntVar(&config.DB.SeedSize, "seed-size", int(DefaultSeedSize), "Minimum candidate budget per source (search widens it to top)")
	flagSet.StringVar(&config.DB.HashingFunction.Name, "hashing-function", DefaultHashingFunction, "Default Hashing function")
	flagSet.Uint64Var(&config.DB.HashingFunction.Seed, "hashing-function-seed", DefaultHashingFunctionSeed, "Hashing function seed")
	flagSet.StringVar(&config.DB.SearchAlgorithm.Name, "search-algorithm", DefaultSearchAlgorithm, "Graph search traversal algorithm")
	flagSet.StringVar(&config.DB.RankingAlgorithm.Name, "ranking-algorithm", DefaultRankingAlgorithm, "Graph search ranking boost")
	flagSet.StringVar(&config.DB.ScoringAlgorithm.Name, "scoring-algorithm", DefaultScoringAlgorithm, "Relevance fold (excess, rrf)")
	flagSet.StringVar(&config.DB.RelevanceModel.Name, "relevance-model", DefaultRelevanceModel, "Text-index relevance model (bm25, matchcount)")
	flagSet.Float64Var(&config.DB.RankingAlgorithm.PageRankDamping, "pagerank-damping", DefaultPageRankDamping, "PageRank damping factor")
	flagSet.IntVar(&config.DB.RankingAlgorithm.PageRankMaxIter, "pagerank-max-iter", DefaultPageRankMaxIter, "PageRank iteration cap")
	flagSet.Float64Var(&config.DB.RankingAlgorithm.PageRankTol, "pagerank-tol", DefaultPageRankTol, "PageRank convergence threshold")

	// vector search
	flagSet.IntVar(&config.DB.VectorSearch.ProjectionDimension, "rptree-projection-dimension", DefaultProjectionDimention, "RP Tree Projection dimension")
	flagSet.IntVar(&config.DB.VectorSearch.NumberTrees, "rptree-n-trees", DefaultNumberTrees, "RP Tree Number Trees")
	flagSet.Uint64Var(&config.DB.VectorSearch.Seed, "rptree-seed", DefaultRPSeed, "RP Tree seed")
	flagSet.IntVar(&config.DB.VectorSearch.FlushFactor, "rptree-flush-factor", DefaultFlushFactor, "RP forest compaction threshold (entries per live vector)")

	return config
}

// Clones config set
func (c *ConfigSet) Clone() *ConfigSet {
	config := &ConfigSet{}
	*config = *c
	return config
}

// Returns config as string (useful for debugging)
func (c *ConfigSet) String() string {
	data, err := json.MarshalIndent(c, "", " ")
	if err != nil {
		return "<nil>"
	}
	return string(data)
}

// / Parses flag definition from argument list.
// priority order is: parameters defined via CLI override config file.
func (c *ConfigSet) Parse(arguments []string) error {

	err := c.FlagSet.Parse(arguments)

	if err != nil {
		return err
	}

	if c.configFile == "" {
		c.configFile = DefaultConfigFile
	}

	// A missing or unreadable config file is survivable — the flags already
	// parsed, plus the built-in defaults, are a complete configuration — so the
	// failure is carried to the end instead of returned here. Returning early
	// used to skip adjust and validate entirely, which is how `-log-level error`
	// with no config file reached the logger unchecked: the very case an
	// operator hits first.
	meta, fileErr := c.FromFile(c.configFile)

	// Parse again to replace with command line options
	err = c.FlagSet.Parse(arguments)
	if err != nil {
		return err
	}

	if len(c.Args()) != 0 {
		return fmt.Errorf("%w: %q", ErrInvalidFlag, c.Arg(0))
	}

	if err = c.adjust(meta); err != nil {
		return err
	}

	// An invalid value outranks a missing file: it is the one the caller must
	// stop on, so it is the one returned.
	if err = c.validate(); err != nil {
		return err
	}

	return fileErr
}

func (c *ConfigSet) adjust(meta *toml.MetaData) error {

	// Reject keys present in the config file that map to no known field.
	if meta != nil {
		if undecoded := meta.Undecoded(); len(undecoded) != 0 {
			return fmt.Errorf("%w: unknown keys %v", ErrParsingFailed, undecoded)
		}
	}

	// scheduler
	Adjust(&c.Scheduler.Workers, max(MinWorkersCount, runtime.GOMAXPROCS(0)))
	Adjust(&c.Scheduler.BufferSize, DefaultBufferSize)
	Adjust(&c.Scheduler.EnqueueTimeout, DefaultEnqueueTimeout)

	// server
	Adjust(&c.Server.Port, DefaultPort)
	Adjust(&c.Server.ReadTimeout, DefaultReadTimeout)
	Adjust(&c.Server.ReadHeaderTimeout, DefaultReadHeaderTimeout)
	Adjust(&c.Server.WriteTimeout, DefaultWriteTimeout)
	Adjust(&c.Server.IdleTimeout, DefaultIdleTimeout)
	Adjust(&c.Server.ShutdownGrace, DefaultShutdownGrace)
	Adjust(&c.Server.MaxBodyBytes, DefaultMaxBodyBytes)

	// log
	Adjust(&c.Log.Level, DefaultLogLevel)
	Adjust(&c.Log.Format, DefaultLogFormat)
	// Log.DisableTimestamp is intentionally not adjusted: its default is true,
	// so Adjust would override any explicit false from the config file.

	// engine
	Adjust(&c.Engine.AllowUnanchoredRecall, DefaultAllowUnanchoredRecall)
	Adjust(&c.Engine.Halflife, DefaultHalflife)
	Adjust(&c.Engine.CacheCapacity, DefaultCacheCapacity)

	// db
	Adjust(&c.DB.NumGraphs, DefaultNumGraph)
	Adjust(&c.DB.DefaultTop, DefaultTop)
	Adjust(&c.DB.DefaultDepth, DefaultDepth)
	Adjust(&c.DB.MaxTop, DefaultMaxTop)
	Adjust(&c.DB.MaxDepth, DefaultMaxDepth)
	Adjust(&c.DB.MaxVectorDimension, DefaultMaxVectorDimension)
	Adjust(&c.DB.Precision, DefaultPrecision)
	Adjust(&c.DB.SeedSize, int(DefaultSeedSize))
	Adjust(&c.DB.HashingFunction.Name, DefaultHashingFunction)
	Adjust(&c.DB.HashingFunction.Seed, DefaultHashingFunctionSeed)
	Adjust(&c.DB.SearchAlgorithm.Name, DefaultSearchAlgorithm)
	Adjust(&c.DB.RankingAlgorithm.Name, DefaultRankingAlgorithm)
	Adjust(&c.DB.ScoringAlgorithm.Name, DefaultScoringAlgorithm)
	Adjust(&c.DB.RelevanceModel.Name, DefaultRelevanceModel)
	Adjust(&c.DB.RankingAlgorithm.PageRankDamping, DefaultPageRankDamping)
	Adjust(&c.DB.RankingAlgorithm.PageRankMaxIter, DefaultPageRankMaxIter)
	Adjust(&c.DB.RankingAlgorithm.PageRankTol, DefaultPageRankTol)

	// vector search
	Adjust(&c.DB.VectorSearch.ProjectionDimension, DefaultProjectionDimention)
	Adjust(&c.DB.VectorSearch.NumberTrees, DefaultNumberTrees)
	Adjust(&c.DB.VectorSearch.Seed, DefaultRPSeed)
	Adjust(&c.DB.VectorSearch.FlushFactor, DefaultFlushFactor)

	return nil
}

func (c *ConfigSet) FromFile(path string) (*toml.MetaData, error) {
	meta, err := toml.DecodeFile(path, c)
	if err != nil {
		return nil, ErrParsingFailed
	}
	return &meta, nil
}
