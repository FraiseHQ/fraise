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

// ConfigSet is the server configuration. It holds one field per section of the
// TOML file, and the flag set whose flags write to the same fields.
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

// SchedulerConfig is the scheduler section. It sets how many workers run
// queries, and how much queued work the scheduler accepts before it turns
// clients away.
type SchedulerConfig struct {
	// Workers is the number of worker goroutines that run reads and writes.
	Workers int `toml:"workers"`

	// BufferSize is the capacity of the scheduler's stream queue.
	BufferSize uint `toml:"buffer-size"`

	// EnqueueTimeout is the maximum time a submit waits for space in a full
	// queue. After it, the request is rejected so clients can back off.
	EnqueueTimeout time.Duration `toml:"enqueue-timeout"`
}

// ServerConfig is the server section. It holds the port the HTTP API listens
// on, and the timeouts and body cap that bound what one client can hold.
type ServerConfig struct {
	Port int `toml:"port"`

	// ReadTimeout is the maximum time to read a whole request, body included.
	ReadTimeout time.Duration `toml:"read-timeout"`

	// ReadHeaderTimeout is the maximum time to read request headers alone. It
	// protects against Slowloris attacks.
	ReadHeaderTimeout time.Duration `toml:"read-header-timeout"`

	// WriteTimeout is the maximum time to write a response.
	WriteTimeout time.Duration `toml:"write-timeout"`

	// IdleTimeout is the maximum time a kept-alive connection may sit idle
	// between requests.
	IdleTimeout time.Duration `toml:"idle-timeout"`

	// ShutdownGrace is how long a graceful shutdown waits for in-flight
	// requests to drain.
	ShutdownGrace time.Duration `toml:"shutdown-grace"`

	// MaxBodyBytes is the maximum request body size, in bytes, that the query
	// endpoints accept.
	MaxBodyBytes int64 `toml:"max-body-bytes"`
}

// LogConfig is the log section. It sets the minimum severity, the line format
// and whether lines carry a timestamp.
type LogConfig struct {
	// Level is the minimum severity logged: DEBUG, INFO, WARN or ERROR. The
	// default is INFO.
	Level string `toml:"level"`

	// Format is the line format, text or json. The default is text. Logs go
	// to stdout, and file logging is not supported.
	Format string `toml:"format"`

	// DisableTimestamp omits the timestamp from every line. The default is
	// false. A supervisor such as journald or docker stamps what it collects,
	// so a second timestamp is noise.
	DisableTimestamp bool `toml:"disable-timestamp"`
}

// EngineConfig is the engine section. It sets the recency decay applied to
// fact scores and the size of the query plan cache.
type EngineConfig struct {
	// Halflife is the half-life of the time decay applied to fact scores.
	Halflife time.Duration `toml:"half-life"`

	// CacheCapacity is how many plans the query plan cache holds.
	CacheCapacity int `toml:"cache-capacity"`
}

// DBConfig is the db section. It holds the store's precision and graph count,
// and the defaults and ceilings a recall is held to. It also names the
// retrieval algorithms, each in a table nested under db.
type DBConfig struct {
	// Precision is the floating-point precision of embeddings and scores,
	// float32 or float64. It selects which generic instantiation of the
	// server is built at startup (see cmd/server).
	Precision string `toml:"precision"`

	// NumGraphs is how many independent graphs the store allocates. Their
	// selectors run from 0 to NumGraphs-1.
	NumGraphs int `toml:"num-graphs"`

	// DefaultTop is the top a recall uses when it has no top clause.
	DefaultTop int `toml:"default-top"`

	// DefaultDepth is the depth a recall uses when it has no depth clause.
	DefaultDepth int `toml:"default-depth"`

	// MaxTop is the ceiling on a recall's top clause. A larger top is
	// rejected at parse time.
	MaxTop int `toml:"max-top"`

	// MaxDepth is the ceiling on a recall's depth clause. A larger depth is
	// rejected at parse time.
	MaxDepth int `toml:"max-depth"`

	// MaxVectorDimension is the ceiling on the length of a bound vector
	// parameter. A longer vector is rejected at parse time.
	MaxVectorDimension int `toml:"max-vector-dimension"`

	// SeedSize is the minimum candidate budget pulled from each source, the
	// text and the vector index. Search widens it to max(seed-size, top), so a
	// recall that asks for more results is never starved of candidates.
	SeedSize int `toml:"seed-size"`

	// MinScoreRatio is the score cutoff. A hit whose relevance is below this
	// fraction of the best relevance in the ranked list is dropped, so a
	// recall stops where the evidence does instead of filling to top.
	// Relevance is the scorer's output before recency decay. Measured after
	// decay, the bar would cut a fact with the same evidence as the best hit
	// for being older. Only the length changes: order and scores are kept, and
	// the best hit always clears its own bar, so a result that matched is
	// never emptied. 0, the default, is off, leaving the trade of recall for
	// precision to the operator. validate rejects anything outside [0, 1].
	MinScoreRatio float64 `toml:"min-score-ratio"`

	// WindowGamma is the window: the share of its temporal neighbours' term
	// counts and length a fact borrows when the text index scores it. Two
	// facts written one after the other under the same anchor are usually one
	// exchange, a question and its answer, and scored alone each is a
	// fragment of it. A fact that holds none of a term but sits beside one
	// that does is a match too. A negative value turns the window off, as a
	// negative half-life turns decay off. 0 means the default. validate
	// rejects anything above 1.
	WindowGamma float64 `toml:"window-gamma"`

	// HashingFunction is the hash that derives node keys from values.
	HashingFunction HashingFunction `toml:"hashing-function"`

	// SearchAlgorithm is the graph traversal a recall runs.
	SearchAlgorithm SearchAlgorithm `toml:"search-algorithm"`

	// ScoringAlgorithm is the relevance fold, excess or rrf.
	ScoringAlgorithm ScoringAlgorithm `toml:"scoring-algorithm"`

	// RelevanceModel is the text index's relevance model, bm25 or matchcount.
	RelevanceModel RelevanceModel `toml:"relevance-model"`

	VectorSearch VectorSearch `toml:"vector-search"`
}

// HashingFunction is the db.hashing-function table. It names the hash that
// derives node keys from values, one of [HashingFunctions], and the seed it
// runs with.
type HashingFunction struct {
	// Name is the hash, xxhash or t1ha.
	Name string `toml:"name"`

	// Seed is the seed the hash runs with.
	Seed uint64 `toml:"seed"`
}

// SearchAlgorithm is the db.search-algorithm table. It names the graph
// traversal a recall runs, one of [SearchAlgorithms].
type SearchAlgorithm struct {
	// Name is the traversal that moves seed evidence through the graph: none,
	// bfs or excess. "none" turns the graph channel off, leaving text and
	// vector search only.
	Name string `toml:"name"`
}

// ScoringAlgorithm is the db.scoring-algorithm table. It names the fold that
// turns a candidate's pooled contributions into its relevance, one of
// [ScoringAlgorithms].
type ScoringAlgorithm struct {
	// Name is the fold, excess or rrf.
	Name string `toml:"name"`
}

// RelevanceModel is the db.relevance-model table. It names the text index's
// relevance model, one of [RelevanceModels].
type RelevanceModel struct {
	// Name is the model, bm25 or matchcount. The model turns a document's
	// match against the query into a number.
	Name string `toml:"name"`
}

// VectorSearch is the db.vector-search table. It shapes the vector index's
// forest of random-projection trees: how many trees, how many random
// directions each splits on, and the seed that makes them reproducible. It
// also sets when leaves split, when searches stop probing and when garbage is
// compacted.
type VectorSearch struct {
	ProjectionDimension int `toml:"projection-dimension"`

	NumberTrees int `toml:"number-trees"`

	Seed uint64 `toml:"seed"`

	// FlushFactor is the forest's garbage compaction threshold, in entries per
	// live vector. Past it, the forest is rebuilt from the live set.
	FlushFactor int `toml:"flush-factor"`

	// LeafSize is how many points an RP-tree leaf holds before it splits into
	// two.
	LeafSize int `toml:"leaf-size"`

	// Overfetch is how many candidates a search gathers per result asked for
	// before it stops probing.
	Overfetch int `toml:"overfetch"`
}

// MCPConfig is the mcp section. The 'fraise mcp' bridge reads it, not the
// daemon it forwards to.
type MCPConfig struct {
	// Address points at the daemon the 'fraise mcp' bridge forwards to. Unset,
	// it is the daemon this same config describes, 127.0.0.1 on server.port.
	// So 'fraise mcp -config x' finds whatever 'fraise -config x' serves. The
	// graph is not set here, because every query names its own with @N.
	Address string `toml:"address"`
}

// New returns a ConfigSet holding the built-in defaults, with a command-line
// flag bound to every setting.
func New() *ConfigSet {
	config := &ConfigSet{}

	config.FlagSet = flag.NewFlagSet("fraise", flag.ContinueOnError)

	flagSet := config.FlagSet

	flagSet.StringVar(&config.configFile, "config", DefaultConfigFile, "Path to the TOML config file")

	defaultWorkers := max(MinWorkersCount, runtime.GOMAXPROCS(0))
	flagSet.IntVar(&config.Scheduler.Workers, "workers", defaultWorkers, "Worker goroutines executing streams")
	flagSet.UintVar(&config.Scheduler.BufferSize, "buffer-size", DefaultBufferSize, "Queue depth: streams waiting for a worker")
	flagSet.DurationVar(&config.Scheduler.EnqueueTimeout, "enqueue-timeout", DefaultEnqueueTimeout, "Max wait for queue space before rejecting a query")

	flagSet.IntVar(&config.Server.Port, "port", DefaultPort, "Server port")
	flagSet.DurationVar(&config.Server.ReadTimeout, "read-timeout", DefaultReadTimeout, "Max time to read a whole request")
	flagSet.DurationVar(&config.Server.ReadHeaderTimeout, "read-header-timeout", DefaultReadHeaderTimeout, "Max time to read request headers")
	flagSet.DurationVar(&config.Server.WriteTimeout, "write-timeout", DefaultWriteTimeout, "Max time to write a response")
	flagSet.DurationVar(&config.Server.IdleTimeout, "idle-timeout", DefaultIdleTimeout, "Max idle time on a kept-alive connection")
	flagSet.DurationVar(&config.Server.ShutdownGrace, "shutdown-grace", DefaultShutdownGrace, "Grace period for in-flight requests on shutdown")
	flagSet.Int64Var(&config.Server.MaxBodyBytes, "max-body-bytes", DefaultMaxBodyBytes, "Max request body size in bytes")

	flagSet.StringVar(&config.Log.Level, "log-level", DefaultLogLevel, "Log level (debug, info, warn, error)")
	flagSet.StringVar(&config.Log.Format, "log-format", DefaultLogFormat, "Log format (text, json)")
	flagSet.BoolVar(&config.Log.DisableTimestamp, "log-disable-timestamp", DefaultLogDisableTimestamp, "Omit the timestamp from every log line")

	flagSet.DurationVar(&config.Engine.Halflife, "half-life", DefaultHalflife, "Time-decay half-life applied to fact scores")
	flagSet.IntVar(&config.Engine.CacheCapacity, "cache-capacity", DefaultCacheCapacity, "Size of the LRU of optimised query plans")

	flagSet.IntVar(&config.DB.NumGraphs, "num-graphs", DefaultNumGraph, "Number of independent graphs the store allocates")
	flagSet.IntVar(&config.DB.DefaultTop, "default-top", DefaultTop, "Results returned when a recall omits top:")
	flagSet.IntVar(&config.DB.DefaultDepth, "default-depth", DefaultDepth, "Retrieval lane when a recall omits depth: (0 = floor)")
	flagSet.IntVar(&config.DB.MaxTop, "max-top", DefaultMaxTop, "Ceiling on a recall's top clause")
	flagSet.IntVar(&config.DB.MaxDepth, "max-depth", DefaultMaxDepth, "Ceiling on a recall's depth clause")
	flagSet.IntVar(&config.DB.MaxVectorDimension, "max-vector-dimension", DefaultMaxVectorDimension, "Ceiling on a bound vector's length")
	flagSet.StringVar(&config.DB.Precision, "precision", DefaultPrecision, "Embedding/score precision: float32 or float64")
	flagSet.IntVar(&config.DB.SeedSize, "seed-size", DefaultSeedSize, "Minimum candidate budget per source (search widens it to top)")
	flagSet.Float64Var(&config.DB.MinScoreRatio, "min-score-ratio", DefaultMinScoreRatio, "Drop hits whose relevance is below this fraction of the best hit's (0 = off)")
	flagSet.Float64Var(&config.DB.WindowGamma, "window-gamma", DefaultWindowGamma, "Share of its temporal neighbours' matches a fact borrows when scored (negative = off)")
	flagSet.StringVar(&config.DB.HashingFunction.Name, "hashing-function", DefaultHashingFunction, "Node-key hashing function (xxhash, t1ha)")
	flagSet.Uint64Var(&config.DB.HashingFunction.Seed, "hashing-function-seed", DefaultHashingFunctionSeed, "Seed of the node-key hashing function")
	flagSet.StringVar(&config.DB.SearchAlgorithm.Name, "search-algorithm", DefaultSearchAlgorithm, "Graph search traversal algorithm")
	flagSet.StringVar(&config.DB.ScoringAlgorithm.Name, "scoring-algorithm", DefaultScoringAlgorithm, "Relevance fold (excess, rrf)")
	flagSet.StringVar(&config.DB.RelevanceModel.Name, "relevance-model", DefaultRelevanceModel, "Text-index relevance model (bm25, matchcount)")

	flagSet.IntVar(&config.DB.VectorSearch.ProjectionDimension, "rptree-projection-dimension", DefaultProjectionDimension, "Dimension each RP-tree projects vectors down to")
	flagSet.IntVar(&config.DB.VectorSearch.NumberTrees, "rptree-n-trees", DefaultNumberTrees, "Trees in the vector index forest")
	flagSet.Uint64Var(&config.DB.VectorSearch.Seed, "rptree-seed", DefaultRPSeed, "Seed of the RP-trees' random projections")
	flagSet.IntVar(&config.DB.VectorSearch.FlushFactor, "rptree-flush-factor", DefaultFlushFactor, "RP forest compaction threshold (entries per live vector)")
	flagSet.IntVar(&config.DB.VectorSearch.LeafSize, "rptree-leaf-size", DefaultLeafSize, "Points an RP-tree leaf holds before it splits")
	flagSet.IntVar(&config.DB.VectorSearch.Overfetch, "rptree-overfetch", DefaultOverfetch, "Candidates gathered per result before a vector search stops probing")

	// The address has no default here. adjust derives it from -port.
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

// Parse resolves the configuration from arguments. It decodes the config file
// they name over the built-in defaults, then applies the flags over the file.
// Settings still at their zero value then take their default, and the result
// is validated. A missing file is ErrMissingFile, returned with the
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

	// A missing config file is survivable, because the flags and the built-in
	// defaults are a complete configuration. Its error is carried to the end,
	// since the flags must still be adjusted and validated. A file that exists
	// but cannot be used stops here. Nothing after it would run with what the
	// operator wrote.
	meta, fileErr := c.FromFile(c.configFile)
	if errors.Is(fileErr, ErrParsingFailed) {
		return fileErr
	}

	// Parse the flags again so they override the file.
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

	// An invalid value outranks a missing file. The caller must stop on it, so
	// it is the one returned. What remains is ErrMissingFile or nil.
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

	Adjust(&c.Scheduler.Workers, max(MinWorkersCount, runtime.GOMAXPROCS(0)))
	Adjust(&c.Scheduler.BufferSize, DefaultBufferSize)
	Adjust(&c.Scheduler.EnqueueTimeout, DefaultEnqueueTimeout)

	Adjust(&c.Server.Port, DefaultPort)
	Adjust(&c.Server.ReadTimeout, DefaultReadTimeout)
	Adjust(&c.Server.ReadHeaderTimeout, DefaultReadHeaderTimeout)
	Adjust(&c.Server.WriteTimeout, DefaultWriteTimeout)
	Adjust(&c.Server.IdleTimeout, DefaultIdleTimeout)
	Adjust(&c.Server.ShutdownGrace, DefaultShutdownGrace)
	Adjust(&c.Server.MaxBodyBytes, DefaultMaxBodyBytes)

	Adjust(&c.Log.Level, DefaultLogLevel)
	Adjust(&c.Log.Format, DefaultLogFormat)
	// Log.DisableTimestamp needs no Adjust. Its default, false, is the zero
	// value, so an absent key and an explicit false already agree.

	// The mcp address is adjusted after server.port, because its default
	// derives from the port.
	Adjust(&c.MCP.Address, fmt.Sprintf(DefaultMCPAddressFormat, c.Server.Port))

	Adjust(&c.Engine.Halflife, DefaultHalflife)
	Adjust(&c.Engine.CacheCapacity, DefaultCacheCapacity)

	Adjust(&c.DB.NumGraphs, DefaultNumGraph)
	Adjust(&c.DB.DefaultTop, DefaultTop)
	// DB.DefaultDepth needs no Adjust. Its default, 0, is the zero value and
	// the floor lane, so an absent key and an explicit 0 already agree.
	Adjust(&c.DB.MaxTop, DefaultMaxTop)
	Adjust(&c.DB.MaxDepth, DefaultMaxDepth)
	Adjust(&c.DB.MaxVectorDimension, DefaultMaxVectorDimension)
	Adjust(&c.DB.Precision, DefaultPrecision)
	Adjust(&c.DB.SeedSize, DefaultSeedSize)
	// DB.MinScoreRatio needs no Adjust. Its default, 0, is the zero value and
	// means off, so an absent key and an explicit 0 already agree.
	Adjust(&c.DB.WindowGamma, DefaultWindowGamma)
	Adjust(&c.DB.HashingFunction.Name, DefaultHashingFunction)
	// DB.HashingFunction.Seed needs no Adjust. Its default, 0, is the zero
	// value, so an absent key and an explicit 0 already agree.
	Adjust(&c.DB.SearchAlgorithm.Name, DefaultSearchAlgorithm)
	Adjust(&c.DB.ScoringAlgorithm.Name, DefaultScoringAlgorithm)
	Adjust(&c.DB.RelevanceModel.Name, DefaultRelevanceModel)

	Adjust(&c.DB.VectorSearch.ProjectionDimension, DefaultProjectionDimension)
	Adjust(&c.DB.VectorSearch.NumberTrees, DefaultNumberTrees)
	Adjust(&c.DB.VectorSearch.Seed, DefaultRPSeed)
	Adjust(&c.DB.VectorSearch.FlushFactor, DefaultFlushFactor)
	Adjust(&c.DB.VectorSearch.LeafSize, DefaultLeafSize)
	Adjust(&c.DB.VectorSearch.Overfetch, DefaultOverfetch)

	return nil
}

// FromFile decodes the TOML file at path over c. A file that does not exist
// is ErrMissingFile, which the caller may survive. Any other failure is
// ErrParsingFailed: the file is unreadable, is not TOML, or holds a value of
// the wrong type. The error names the file and what was wrong with it.
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
