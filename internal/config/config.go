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
}

type ServerConfig struct {
	Port int `toml:"port"`
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

	// default top
	DefaultTop int `toml:"default-top"`

	// default depth
	DefaultDepth int `toml:"default-depth"`

	// How many seeds to pull from each source (keywords and vector)
	SeedSize int `toml:"seed-size"`

	// Score attenuation for graph walk
	HopAttenuation float64 `toml:"hop-attenuation"`

	// database hashing function
	HashingFunction HashingFunction `toml:"hashing-function"`

	// graph search traversal algorithm
	SearchAlgorithm SearchAlgorithm `toml:"search-algorithm"`

	// graph search ranking boost (none, pagerank)
	RankingAlgorithm RankingAlgorithm `toml:"ranking-algorithm"`

	VectorSearch VectorSearch `toml:"vector-search"`
}

type HashingFunction struct {
	// (xxhash, t1ha)
	Name string `toml:"name"`

	// hashing function seed
	Seed uint64 `toml:"seed"`
}

type SearchAlgorithm struct {
	// name (bfs)
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
}

// Instanciates new configset
func New() *ConfigSet {
	config := &ConfigSet{}

	config.FlagSet = flag.NewFlagSet("flags", flag.PanicOnError)

	flagSet := config.FlagSet

	// config file (path to the TOML config Parse reads before applying flags)
	flagSet.StringVar(&config.configFile, "config", DefaultConfigFile, "Path to the TOML config file")

	// scheduler
	flagSet.IntVar(&config.Scheduler.Workers, "workers", DefaultWorkersCount, "Default worker count.")
	flagSet.UintVar(&config.Scheduler.BufferSize, "buffer-size", DefaultBufferSize, "Default Buffer size")

	// server
	flagSet.IntVar(&config.Server.Port, "port", DefaultPort, "Server port")

	// log
	flagSet.StringVar(&config.Log.Level, "log-level", DefaultLogLevel, "Log level")
	flagSet.StringVar(&config.Log.Format, "log-format", DefaultLogFormat, "Log Format")
	flagSet.BoolVar(&config.Log.DisableTimestamp, "log-disable-timestamp", true, "Log Format")

	// engine
	flagSet.BoolVar(&config.Engine.AllowUnanchoredRecall, "allow-unanchored-recall", DefaultAllowUnanchoredRecall, "Allow unanchored recalls")
	flagSet.DurationVar(&config.Engine.Halflife, "half-life", DefaultHalflife, "Half life for time decay")
	flagSet.IntVar(&config.Engine.CacheCapacity, "cache-capacity", DefaultCacheCapacity, "Query cache size")

	// db
	flagSet.IntVar(&config.DB.DefaultTop, "default-top", DefaultTop, "Default Top")
	flagSet.IntVar(&config.DB.DefaultDepth, "default-depth", DefaultDepth, "Default Depth")
	flagSet.StringVar(&config.DB.Precision, "precision", DefaultPrecision, "Embedding/score precision: float32 or float64")
	flagSet.IntVar(&config.DB.SeedSize, "seed-size", int(DefaultSeedSize), "Seeds to pull from each source")
	flagSet.Float64Var(&config.DB.HopAttenuation, "hop-attenuation", float64(DefaultHopAttenuation), "Score attenuation for graph walk")
	flagSet.StringVar(&config.DB.HashingFunction.Name, "hashing-function", DefaultHashingFunction, "Default Hashing function")
	flagSet.Uint64Var(&config.DB.HashingFunction.Seed, "hashing-function-seed", DefaultHashingFunctionSeed, "Hashing function seed")
	flagSet.StringVar(&config.DB.SearchAlgorithm.Name, "search-algorithm", DefaultSearchAlgorithm, "Graph search traversal algorithm")
	flagSet.StringVar(&config.DB.RankingAlgorithm.Name, "ranking-algorithm", DefaultRankingAlgorithm, "Graph search ranking boost")
	flagSet.Float64Var(&config.DB.RankingAlgorithm.PageRankDamping, "pagerank-damping", DefaultPageRankDamping, "PageRank damping factor")
	flagSet.IntVar(&config.DB.RankingAlgorithm.PageRankMaxIter, "pagerank-max-iter", DefaultPageRankMaxIter, "PageRank iteration cap")
	flagSet.Float64Var(&config.DB.RankingAlgorithm.PageRankTol, "pagerank-tol", DefaultPageRankTol, "PageRank convergence threshold")

	// vector search
	flagSet.IntVar(&config.DB.VectorSearch.ProjectionDimension, "rptree-projection-dimension", DefaultProjectionDimention, "RP Tree Projection dimension")
	flagSet.IntVar(&config.DB.VectorSearch.NumberTrees, "rptree-n-trees", DefaultNumberTrees, "RP Tree Number Trees")
	flagSet.Uint64Var(&config.DB.VectorSearch.Seed, "rptree-seed", DefaultRPSeed, "RP Tree seed")

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

	var meta *toml.MetaData
	meta, err = c.FromFile(c.configFile)

	if err != nil {
		return err
	}

	// Parse again to replace with command line options
	err = c.FlagSet.Parse(arguments)
	if err != nil {
		return err
	}

	if len(c.Args()) != 0 {
		return fmt.Errorf("'%s' is an invalid flag", c.Arg(0))
	}

	err = c.adjust(meta)

	return err
}

func (c *ConfigSet) adjust(meta *toml.MetaData) error {

	// Reject keys present in the config file that map to no known field.
	if meta != nil {
		if undecoded := meta.Undecoded(); len(undecoded) != 0 {
			return fmt.Errorf("%w: unknown keys %v", ErrParsingFailed, undecoded)
		}
	}

	// scheduler
	Adjust(&c.Scheduler.Workers, DefaultWorkersCount)
	Adjust(&c.Scheduler.BufferSize, DefaultBufferSize)

	// server
	Adjust(&c.Server.Port, DefaultPort)

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
	Adjust(&c.DB.DefaultTop, DefaultTop)
	Adjust(&c.DB.DefaultDepth, DefaultDepth)
	Adjust(&c.DB.Precision, DefaultPrecision)
	Adjust(&c.DB.SeedSize, int(DefaultSeedSize))
	Adjust(&c.DB.HopAttenuation, float64(DefaultHopAttenuation))
	Adjust(&c.DB.HashingFunction.Name, DefaultHashingFunction)
	Adjust(&c.DB.HashingFunction.Seed, DefaultHashingFunctionSeed)
	Adjust(&c.DB.SearchAlgorithm.Name, DefaultSearchAlgorithm)
	Adjust(&c.DB.RankingAlgorithm.Name, DefaultRankingAlgorithm)
	Adjust(&c.DB.RankingAlgorithm.PageRankDamping, DefaultPageRankDamping)
	Adjust(&c.DB.RankingAlgorithm.PageRankMaxIter, DefaultPageRankMaxIter)
	Adjust(&c.DB.RankingAlgorithm.PageRankTol, DefaultPageRankTol)

	// vector search
	Adjust(&c.DB.VectorSearch.ProjectionDimension, DefaultProjectionDimention)
	Adjust(&c.DB.VectorSearch.NumberTrees, DefaultNumberTrees)
	Adjust(&c.DB.VectorSearch.Seed, DefaultRPSeed)

	return nil
}

func (c *ConfigSet) FromFile(path string) (*toml.MetaData, error) {
	meta, err := toml.DecodeFile(path, c)
	if err != nil {
		return nil, ErrParsingFailed
	}
	return &meta, nil
}
