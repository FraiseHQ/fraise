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

import "time"

const (
	// DefaultConfigFile is the path the server reads configuration from when no
	// -config flag is given.
	DefaultConfigFile = "fraise.config.toml"

	// DefaultPort is the TCP port the HTTP API listens on.
	DefaultPort = 9876

	// DefaultReadTimeout bounds how long the server will spend reading a whole
	// request (headers + body). It stops a slow client from holding a
	// connection open indefinitely.
	DefaultReadTimeout time.Duration = 15 * time.Second

	// DefaultReadHeaderTimeout bounds how long the server waits for request
	// headers alone; it caps slow-header (Slowloris-style) connections.
	DefaultReadHeaderTimeout time.Duration = 5 * time.Second

	// DefaultWriteTimeout bounds how long a response may take to write before the
	// connection is torn down.
	DefaultWriteTimeout time.Duration = 15 * time.Second

	// DefaultIdleTimeout bounds how long a kept-alive connection may sit idle
	// between requests before it is closed.
	DefaultIdleTimeout time.Duration = 60 * time.Second

	// DefaultShutdownGrace is how long a graceful shutdown waits for in-flight
	// requests (and the writes they triggered) to finish before forcing exit.
	DefaultShutdownGrace time.Duration = 10 * time.Second

	// DefaultMaxBodyBytes caps the size of a request body the query endpoints
	// will read. A larger body is rejected before it is buffered, bounding
	// memory.
	DefaultMaxBodyBytes int64 = 1 << 20 // 1 MiB

	// DefaultNumGraph is how many independent graphs the store allocates; valid
	// selectors are 0..DefaultNumGraph-1. Graph selectors are uint8, so
	// validation rejects a count above 256: the extra graphs would be
	// allocated and never reachable.
	DefaultNumGraph int = 8

	// MinWorkersCount is the floor of the default scheduler worker count,
	// max(MinWorkersCount, runtime.GOMAXPROCS(0)). Reads take RLock and run
	// concurrently, so fewer workers than cores queues reads for nothing,
	// while more workers than cores buys nothing.
	MinWorkersCount int = 2

	// DefaultBufferSize is the capacity of the scheduler's stream queue.
	DefaultBufferSize uint = 200

	// DefaultEnqueueTimeout bounds how long a submit waits for space in a full
	// stream queue before the request is rejected, so a saturated scheduler
	// sheds load instead of parking handler goroutines without bound.
	DefaultEnqueueTimeout time.Duration = 2 * time.Second

	// DefaultLogLevel is the minimum log level emitted. It names one of the
	// accepted spellings in validate.go rather than repeating a literal, so the
	// default cannot drift from the values validate accepts.
	DefaultLogLevel string = LogLevelInfo

	// DefaultLogFormat is the log output format.
	DefaultLogFormat string = LogFormatText

	// DefaultLogDisableTimestamp keeps timestamps on: a log read from a file
	// or a terminal has no other clock, and a supervisor that stamps its own
	// lines is the case that opts out.
	DefaultLogDisableTimestamp bool = false

	// DefaultHashingFunction is the hash used to derive node keys from values.
	DefaultHashingFunction string = HashingXxhash

	// DefaultHashingFunctionSeed seeds the node-key hashing function.
	DefaultHashingFunctionSeed uint64 = 0

	// DefaultSearchAlgorithm is the traversal moving seed evidence through the
	// graph; excess transmission is the shipped methodology, "bfs" remains
	// available for comparison runs, and "none" turns the graph channel off
	// entirely (text/vector search only).
	DefaultSearchAlgorithm string = SearchExcess

	// DefaultScoringAlgorithm selects scoring.ExcessScorer, the shipped
	// scoring methodology; "rrf" selects scoring.RRFScorer, kept for
	// comparison runs.
	DefaultScoringAlgorithm string = ScoringExcess

	// DefaultRelevanceModel is the text index's relevance model; the excess
	// methodology needs BM25's raw retrieval mass, and "matchcount" remains
	// available for comparison runs.
	DefaultRelevanceModel string = RelevanceBM25

	// DefaultRankingAlgorithm is the global ranking boost applied to relevance
	// scores; "none" disables it (the alternative is "pagerank").
	DefaultRankingAlgorithm string = RankingNone

	// DefaultPageRankDamping is the PageRank damping factor (used when ranking is
	// "pagerank").
	DefaultPageRankDamping float64 = 0.85

	// DefaultPageRankMaxIter caps the number of PageRank power-iteration steps.
	DefaultPageRankMaxIter int = 100

	// DefaultPageRankTol is the convergence threshold that stops PageRank early.
	DefaultPageRankTol float64 = 1e-6

	// DefaultTop is how many ranked results a recall returns when no top clause
	// is given.
	DefaultTop int = 10

	// DefaultDepth is the depth a recall uses when no depth clause is given: 0,
	// the floor lane, which ranks by seed mass alone and skips the anchor
	// traversal. A recall naming a topic or entity opts into the traversal
	// with depth:1 (the precision lane: only strongly above-chance anchors
	// transmit) or depth:2 (max recall: any anchor above its fair share
	// transmits).
	DefaultDepth int = 0

	// DefaultMaxTop is the ceiling on a recall's top clause. A request asking for
	// more than this many results is rejected at parse time, so a single query
	// cannot force an unbounded result set.
	DefaultMaxTop int = 1000

	// DefaultMaxDepth is the ceiling on a recall's depth clause. The three
	// lanes are depth 0 (floor), depth 1 (precision) and depth 2 (max recall).
	// Search runs a single anchor-mediated round, so a deeper request would
	// behave exactly like depth 2; it is rejected at parse time instead.
	DefaultMaxDepth int = 2

	// DefaultMaxVectorDimension is the ceiling on the length of a bound vector
	// parameter. A longer vector is rejected at parse time, bounding the work an
	// index insert or search can be asked to do.
	DefaultMaxVectorDimension int = 4096

	// DefaultHalflife is the time-decay half-life applied to fact scores.
	DefaultHalflife time.Duration = 7 * 24 * time.Hour

	// DefaultSeedSize is the minimum candidate budget each source (text and
	// vector index) contributes to a search; the effective budget is
	// max(seed-size, top), so a large recall is never starved of candidates.
	DefaultSeedSize int = 10

	// DefaultMinScoreRatio leaves the score cutoff (DBConfig.MinScoreRatio)
	// off.
	DefaultMinScoreRatio float64 = 0

	// DefaultCacheCapacity is the size of the LRU cache of optimised query plans.
	DefaultCacheCapacity int = 1000

	// DefaultProjectionDimension is the dimension vectors are randomly projected
	// down to inside each RP-tree. It is the number of split directions a tree
	// can draw on, so a narrow projection makes every level of a deep tree reuse
	// the same few directions and the partition stops resembling the space.
	// Ingest does not pay for it: a split names one row and routing reads one
	// row (Projection.ApplyRow).
	DefaultProjectionDimension int = 128

	// DefaultNumberTrees is how many RP-trees form the vector index forest. A
	// single tree is a weak approximator, and the forest makes up for it with
	// independent projections, so the tree count is the setting vector recall
	// depends on most: measured on 50k 128-d vectors, going from 4 to 16 trees
	// roughly quadruples recall@10 at every projection dimension. Query cost,
	// write cost and memory grow linearly with it; the default favours recall
	// over latency.
	DefaultNumberTrees int = 16

	// DefaultRPSeed seeds the RP-trees' random projections (deterministic builds).
	DefaultRPSeed uint64 = 4

	// DefaultLeafSize is how many points an RP-tree leaf accumulates before it
	// splits into two. It sets the granularity of the partition and so the floor
	// on what a single probe examines: larger leaves mean fewer, coarser regions
	// and more candidates scanned per probe, smaller leaves the reverse. It is
	// the least useful of the vector settings to move, since overfetch reaches
	// the same candidate counts without rebuilding the index.
	DefaultLeafSize int = 32

	// DefaultOverfetch is how many candidates a vector search gathers per result
	// asked for before it stops probing. A pool of exactly the requested size
	// leaves the true-distance re-ranking nothing to choose between, so this is
	// what converts probing into recall; at the limit it converges on an exact
	// scan. It shapes no index, so it costs query time only, nothing in write
	// cost or resident memory. Recall per unit of query time stays flat as it
	// rises, so it is a budget choice rather than an optimum. Measured on 50k
	// uniform 128-d vectors, recall@10 runs 0.12 at 8, 0.24 at 16, 0.42 at 32
	// and 0.53 at 64.
	DefaultOverfetch int = 32

	// DefaultFlushFactor bounds RP forest garbage: once a tree holds more than
	// this many entries per live vector, the forest is rebuilt from the live set.
	DefaultFlushFactor int = 2

	// DefaultPrecision is the floating-point precision for embeddings and scores
	// ("float32" or "float64"), selecting which server instantiation is built.
	DefaultPrecision string = PrecisionFloat32

	// DefaultMCPAddressFormat is the address the mcp bridge forwards to when
	// none is configured: the local daemon, on whatever port the same config
	// gives it.
	DefaultMCPAddressFormat string = "http://127.0.0.1:%d"
)
