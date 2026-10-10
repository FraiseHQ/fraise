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
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"runtime"
	"time"

	"github.com/BurntSushi/toml"
)

// ConfigSet is the server configuration: one field per section of the TOML
// file, and the flag set whose flags write to the same fields.
type ConfigSet struct {
	*flag.FlagSet `json:"-"`

	Scheduler SchedulerConfig `toml:"scheduler"`
	Server    ServerConfig    `toml:"server"`
	Log       LogConfig       `toml:"log"`
	Engine    EngineConfig    `toml:"engine"`
	DB        DBConfig        `toml:"db"`
	MCP       MCPConfig       `toml:"mcp"`

	configFile string
}

// SchedulerConfig is the scheduler section: how many workers run queries and
// how much queued work the scheduler accepts before it turns clients away.
type SchedulerConfig struct {
	// Number of worker goroutines executing reads and writes.
	Workers int `toml:"workers"`

	// Capacity of the scheduler's stream queue.
	BufferSize uint `toml:"buffer-size"`

	// Maximum time a submit waits for space in a full queue before the
	// request is rejected so clients can back off.
	EnqueueTimeout time.Duration `toml:"enqueue-timeout"`
}

// ServerConfig is the server section: the port the HTTP API listens on, and
// the timeouts and body cap that bound what a single client can hold.
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

	// Maximum request body size, in bytes, accepted by the query endpoints.
	MaxBodyBytes int64 `toml:"max-body-bytes"`
}

// LogConfig is the log section: the minimum severity, the line format and
// whether lines carry a timestamp.
type LogConfig struct {
	// LOG LEVEL: DEBUG, INFO, WARN, ERROR (default = INFO)
	Level string `toml:"level"`

	// LOG FORMAT: text or json (default = text)
	// Logs go to stdout; file logging is not supported.
	Format string `toml:"format"`

	// Omit the timestamp from every line (default = false). For a supervisor
	// that stamps what it collects — journald, docker — a second time is noise.
	DisableTimestamp bool `toml:"disable-timestamp"`
}

// EngineConfig is the engine section: the recency decay applied to fact
// scores and the size of the query plan cache.
type EngineConfig struct {
	// Half life for time decay (used to score facts)
	Halflife time.Duration `toml:"half-life"`

	// Query cache size
	CacheCapacity int `toml:"cache-capacity"`
}

// DBConfig is the db section: the store's precision and graph count, the
// defaults and ceilings a recall is held to, and the retrieval algorithms,
// each chosen by name in a table nested under db.
type DBConfig struct {
	// Floating-point precision for embeddings and scores: "float32" or
	// "float64". Selects which generic instantiation of the server is built at
	// startup (see cmd/server).
	Precision string `toml:"precision"`

	// How many independent graphs the store allocates (selectors 0..n-1)
	NumGraphs int `toml:"num-graphs"`

	// Top a recall uses when it has no top clause.
	DefaultTop int `toml:"default-top"`

	// Depth a recall uses when it has no depth clause.
	DefaultDepth int `toml:"default-depth"`

	// Ceiling on a recall's top clause (rejected past this at parse time).
	MaxTop int `toml:"max-top"`

	// Ceiling on a recall's depth clause (rejected past this at parse time).
	MaxDepth int `toml:"max-depth"`

	// Ceiling on the length of a bound vector parameter (rejected past this at
	// parse time).
	MaxVectorDimension int `toml:"max-vector-dimension"`

	// The *minimum* candidate budget pulled from each source (text and vector
	// index). Search widens it to max(seed-size, top), so a recall asking for
	// more results than this is never starved of candidates.
	SeedSize int `toml:"seed-size"`

	// Score cutoff: a hit whose relevance is below this fraction of the best
	// relevance in the ranked list is dropped, so a recall stops where the
	// evidence does instead of filling to top. Relevance is the scorer's
	// output before the ranking boost and recency decay; measured after
	// decay, the bar would cut a fact with the same evidence as the best hit
	// for being older. Only the length changes: order and scores are kept,
	// and the best hit always clears its own bar, so a result that matched is
	// never emptied. 0 (the default) is off, leaving the trade of recall for
	// precision to the operator; validate rejects anything outside [0, 1].
	MinScoreRatio float64 `toml:"min-score-ratio"`

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

// HashingFunction is the db.hashing-function table: the hash that derives node
// keys from values, one of [HashingFunctions], and the seed it runs with.
type HashingFunction struct {
	// (xxhash, t1ha)
	Name string `toml:"name"`

	// hashing function seed
	Seed uint64 `toml:"seed"`
}

// SearchAlgorithm is the db.search-algorithm table, naming the graph traversal
// recall runs, one of [SearchAlgorithms].
type SearchAlgorithm struct {
	// name (none, bfs, excess): the traversal moving seed evidence through
	// the graph; "none" turns the graph channel off (text/vector only)
	Name string `toml:"name"`
}

// ScoringAlgorithm is the db.scoring-algorithm table, naming the fold that
// turns a candidate's pooled contributions into its relevance, one of
// [ScoringAlgorithms].
type ScoringAlgorithm struct {
	// name (excess, rrf): the fold deriving each candidate's relevance from
	// its pooled contributions
	Name string `toml:"name"`
}

// RelevanceModel is the db.relevance-model table, naming the text index's
// relevance model, one of [RelevanceModels].
type RelevanceModel struct {
	// name (bm25, matchcount): the text index's relevance model — how a
	// document's match against the query becomes a number
	Name string `toml:"name"`
}

// RankingAlgorithm is the db.ranking-algorithm table: the global boost
// applied to relevance, one of [RankingAlgorithms], and the PageRank
// parameters, which apply only when that boost is pagerank.
type RankingAlgorithm struct {
	// none or pagerank
	Name string `toml:"name"`

	// PageRank probability of following an edge (used when
	// ranking-algorithm is pagerank)
	PageRankDamping float64 `toml:"pagerank-damping"`

	// PageRank iteration cap
	PageRankMaxIter int `toml:"pagerank-max-iter"`

	// PageRank convergence threshold on the score delta
	PageRankTol float64 `toml:"pagerank-tol"`
}

// VectorSearch is the db.vector-search table, shaping the vector index's forest
// of random-projection trees: how many trees, how many random directions each
// splits on, the seed that makes them reproducible, and when leaves split,
// searches stop probing and garbage is compacted.
type VectorSearch struct {
	ProjectionDimension int `toml:"projection-dimension"`

	NumberTrees int `toml:"number-trees"`

	Seed uint64 `toml:"seed"`

	// Forest garbage compaction threshold: entries per live vector before
	// the forest is rebuilt from the live set.
	FlushFactor int `toml:"flush-factor"`

	// Points an RP-tree leaf holds before it splits into two.
	LeafSize int `toml:"leaf-size"`

	// Candidates gathered per result asked for before a search stops probing.
	Overfetch int `toml:"overfetch"`
}

// MCPConfig is the mcp section, read by the 'fraise mcp' bridge rather than by
// the daemon it forwards to.
type MCPConfig struct {
	// Address of the daemon the 'fraise mcp' bridge forwards to. Unset, it is
	// the daemon this same config describes (127.0.0.1 on server.port), so
	// 'fraise mcp -config x' finds whatever 'fraise -config x' serves. The
	// graph is not configured here: every query names its own with @N.
	Address string `toml:"address"`
}

// New returns a ConfigSet holding the built-in defaults, with a command-line
// flag bound to every setting.
func New() *ConfigSet {
	config := &ConfigSet{}

	config.FlagSet = flag.NewFlagSet("fraise", flag.ContinueOnError)

	flagSet := config.FlagSet

	// config file (path to the TOML config Parse reads before applying flags)
	flagSet.StringVar(&config.configFile, "config", DefaultConfigFile, "Path to the TOML config file")

	// scheduler
	defaultWorkers := max(MinWorkersCount, runtime.GOMAXPROCS(0))
	flagSet.IntVar(&config.Scheduler.Workers, "workers", defaultWorkers, "Worker goroutines executing streams")
	flagSet.UintVar(&config.Scheduler.BufferSize, "buffer-size", DefaultBufferSize, "Queue depth: streams waiting for a worker")
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
	flagSet.StringVar(&config.Log.Level, "log-level", DefaultLogLevel, "Log level (debug, info, warn, error)")
	flagSet.StringVar(&config.Log.Format, "log-format", DefaultLogFormat, "Log format (text, json)")
	flagSet.BoolVar(&config.Log.DisableTimestamp, "log-disable-timestamp", DefaultLogDisableTimestamp, "Omit the timestamp from every log line")

	// engine
	flagSet.DurationVar(&config.Engine.Halflife, "half-life", DefaultHalflife, "Time-decay half-life applied to fact scores")
	flagSet.IntVar(&config.Engine.CacheCapacity, "cache-capacity", DefaultCacheCapacity, "Size of the LRU of optimised query plans")

	// db
	flagSet.IntVar(&config.DB.NumGraphs, "num-graphs", DefaultNumGraph, "Number of independent graphs the store allocates")
	flagSet.IntVar(&config.DB.DefaultTop, "default-top", DefaultTop, "Results returned when a recall omits top:")
	flagSet.IntVar(&config.DB.DefaultDepth, "default-depth", DefaultDepth, "Retrieval lane when a recall omits depth: (0 = floor)")
	flagSet.IntVar(&config.DB.MaxTop, "max-top", DefaultMaxTop, "Ceiling on a recall's top clause")
	flagSet.IntVar(&config.DB.MaxDepth, "max-depth", DefaultMaxDepth, "Ceiling on a recall's depth clause")
	flagSet.IntVar(&config.DB.MaxVectorDimension, "max-vector-dimension", DefaultMaxVectorDimension, "Ceiling on a bound vector's length")
	flagSet.StringVar(&config.DB.Precision, "precision", DefaultPrecision, "Embedding/score precision: float32 or float64")
	flagSet.IntVar(&config.DB.SeedSize, "seed-size", DefaultSeedSize, "Minimum candidate budget per source (search widens it to top)")
	flagSet.Float64Var(&config.DB.MinScoreRatio, "min-score-ratio", DefaultMinScoreRatio, "Drop hits whose relevance is below this fraction of the best hit's (0 = off)")
	flagSet.StringVar(&config.DB.HashingFunction.Name, "hashing-function", DefaultHashingFunction, "Node-key hashing function (xxhash, t1ha)")
	flagSet.Uint64Var(&config.DB.HashingFunction.Seed, "hashing-function-seed", DefaultHashingFunctionSeed, "Seed of the node-key hashing function")
	flagSet.StringVar(&config.DB.SearchAlgorithm.Name, "search-algorithm", DefaultSearchAlgorithm, "Graph search traversal algorithm")
	flagSet.StringVar(&config.DB.RankingAlgorithm.Name, "ranking-algorithm", DefaultRankingAlgorithm, "Graph search ranking boost")
	flagSet.StringVar(&config.DB.ScoringAlgorithm.Name, "scoring-algorithm", DefaultScoringAlgorithm, "Relevance fold (excess, rrf)")
	flagSet.StringVar(&config.DB.RelevanceModel.Name, "relevance-model", DefaultRelevanceModel, "Text-index relevance model (bm25, matchcount)")
	flagSet.Float64Var(&config.DB.RankingAlgorithm.PageRankDamping, "pagerank-damping", DefaultPageRankDamping, "PageRank damping factor")
	flagSet.IntVar(&config.DB.RankingAlgorithm.PageRankMaxIter, "pagerank-max-iter", DefaultPageRankMaxIter, "PageRank iteration cap")
	flagSet.Float64Var(&config.DB.RankingAlgorithm.PageRankTol, "pagerank-tol", DefaultPageRankTol, "PageRank convergence threshold")

	// vector search
	flagSet.IntVar(&config.DB.VectorSearch.ProjectionDimension, "rptree-projection-dimension", DefaultProjectionDimension, "Dimension each RP-tree projects vectors down to")
	flagSet.IntVar(&config.DB.VectorSearch.NumberTrees, "rptree-n-trees", DefaultNumberTrees, "Trees in the vector index forest")
	flagSet.Uint64Var(&config.DB.VectorSearch.Seed, "rptree-seed", DefaultRPSeed, "Seed of the RP-trees' random projections")
	flagSet.IntVar(&config.DB.VectorSearch.FlushFactor, "rptree-flush-factor", DefaultFlushFactor, "RP forest compaction threshold (entries per live vector)")
	flagSet.IntVar(&config.DB.VectorSearch.LeafSize, "rptree-leaf-size", DefaultLeafSize, "Points an RP-tree leaf holds before it splits")
	flagSet.IntVar(&config.DB.VectorSearch.Overfetch, "rptree-overfetch", DefaultOverfetch, "Candidates gathered per result before a vector search stops probing")

	// mcp bridge (unset: derived from -port in adjust)
	flagSet.StringVar(&config.MCP.Address, "addr", "", "Address of the fraise daemon the mcp bridge forwards to (default: 127.0.0.1 on -port)")

	return config
}

// Clone returns a shallow copy of c.
func (c *ConfigSet) Clone() *ConfigSet {
	config := &ConfigSet{}
	*config = *c
	return config
}

// String returns the configuration as indented JSON, for debug logging.
func (c *ConfigSet) String() string {
	data, err := json.MarshalIndent(c, "", " ")
	if err != nil {
		return "<nil>"
	}
	return string(data)
}

// Parse resolves the configuration from arguments. The config file they name
// is decoded over the built-in defaults and the flags are applied over the
// file; settings still at their zero value then take their default, and the
// result is validated. A missing file is ErrMissingFile, returned with the
// configuration complete and valid. flag.ErrHelp, ErrInvalidFlag,
// ErrParsingFailed and ErrInvalidValue mean the configuration must not be
// used.
func (c *ConfigSet) Parse(arguments []string) error {

	err := c.FlagSet.Parse(arguments)

	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return fmt.Errorf("%w: %v", ErrInvalidFlag, err)
	}

	if c.configFile == "" {
		c.configFile = DefaultConfigFile
	}

	// A missing config file is survivable, since the flags and the built-in
	// defaults are a complete configuration, so its error is carried to the
	// end: the flags must still be adjusted and validated. A file that exists
	// but cannot be used stops here, since nothing after it would run with
	// what the operator wrote.
	meta, fileErr := c.FromFile(c.configFile)
	if errors.Is(fileErr, ErrParsingFailed) {
		return fileErr
	}

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
	// stop on, so it is the one returned. What remains is ErrMissingFile or nil.
	if err = c.validate(); err != nil {
		return err
	}

	return fileErr
}

func (c *ConfigSet) adjust(meta *toml.MetaData) error {

	// Reject keys present in the config file that map to no known field.
	if meta != nil {
		if undecoded := meta.Undecoded(); len(undecoded) != 0 {
			return fmt.Errorf("%w: %s: unknown keys %v", ErrParsingFailed, c.configFile, undecoded)
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
	// Log.DisableTimestamp needs no Adjust: its default, false, is the zero
	// value, so an absent key and an explicit false already agree.

	// mcp: after server.port, which the default address is derived from
	Adjust(&c.MCP.Address, fmt.Sprintf(DefaultMCPAddressFormat, c.Server.Port))

	// engine
	Adjust(&c.Engine.Halflife, DefaultHalflife)
	Adjust(&c.Engine.CacheCapacity, DefaultCacheCapacity)

	// db
	Adjust(&c.DB.NumGraphs, DefaultNumGraph)
	Adjust(&c.DB.DefaultTop, DefaultTop)
	// DB.DefaultDepth needs no Adjust: its default, 0, is the zero value and
	// the floor lane, so an absent key and an explicit 0 already agree.
	Adjust(&c.DB.MaxTop, DefaultMaxTop)
	Adjust(&c.DB.MaxDepth, DefaultMaxDepth)
	Adjust(&c.DB.MaxVectorDimension, DefaultMaxVectorDimension)
	Adjust(&c.DB.Precision, DefaultPrecision)
	Adjust(&c.DB.SeedSize, DefaultSeedSize)
	// DB.MinScoreRatio needs no Adjust: its default, 0, is the zero value and
	// means off, so an absent key and an explicit 0 already agree.
	Adjust(&c.DB.HashingFunction.Name, DefaultHashingFunction)
	// DB.HashingFunction.Seed needs no Adjust: its default, 0, is the zero
	// value, so an absent key and an explicit 0 already agree.
	Adjust(&c.DB.SearchAlgorithm.Name, DefaultSearchAlgorithm)
	Adjust(&c.DB.RankingAlgorithm.Name, DefaultRankingAlgorithm)
	Adjust(&c.DB.ScoringAlgorithm.Name, DefaultScoringAlgorithm)
	Adjust(&c.DB.RelevanceModel.Name, DefaultRelevanceModel)
	Adjust(&c.DB.RankingAlgorithm.PageRankDamping, DefaultPageRankDamping)
	Adjust(&c.DB.RankingAlgorithm.PageRankMaxIter, DefaultPageRankMaxIter)
	Adjust(&c.DB.RankingAlgorithm.PageRankTol, DefaultPageRankTol)

	// vector search
	Adjust(&c.DB.VectorSearch.ProjectionDimension, DefaultProjectionDimension)
	Adjust(&c.DB.VectorSearch.NumberTrees, DefaultNumberTrees)
	Adjust(&c.DB.VectorSearch.Seed, DefaultRPSeed)
	Adjust(&c.DB.VectorSearch.FlushFactor, DefaultFlushFactor)
	Adjust(&c.DB.VectorSearch.LeafSize, DefaultLeafSize)
	Adjust(&c.DB.VectorSearch.Overfetch, DefaultOverfetch)

	return nil
}

// FromFile decodes the TOML file at path over c. A file that does not exist
// is ErrMissingFile, which the caller may survive; any other failure — the
// file is unreadable, not TOML, or holds a value of the wrong type — is
// ErrParsingFailed, naming the file and what was wrong with it.
func (c *ConfigSet) FromFile(path string) (*toml.MetaData, error) {
	meta, err := toml.DecodeFile(path, c)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrMissingFile, path)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrParsingFailed, path, err)
	}
	return &meta, nil
}
