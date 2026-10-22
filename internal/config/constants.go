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

	// DefaultReadTimeout bounds how long the server spends reading a whole
	// request (headers and body). It stops a slow client from holding a
	// connection open indefinitely.
	DefaultReadTimeout time.Duration = 15 * time.Second

	// DefaultReadHeaderTimeout bounds how long the server waits for request
	// headers alone. It caps slow-header (Slowloris) connections.
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
	// read. A larger body is rejected before it is buffered, bounding memory.
	DefaultMaxBodyBytes int64 = 1 << 20 // 1 MiB

	// DefaultNumGraph is how many independent graphs the store allocates.
	// Valid selectors are 0..DefaultNumGraph-1. Graph selectors are uint8, so
	// validation rejects a count above 256. The extra graphs would be
	// allocated but never reachable.
	DefaultNumGraph int = 8

	// MinWorkersCount is the floor of the default scheduler worker count,
	// max(MinWorkersCount, runtime.GOMAXPROCS(0)). Reads take RLock and run
	// concurrently. Fewer workers than cores queues reads for nothing, and
	// more workers than cores buys nothing.
	MinWorkersCount int = 2

	// DefaultBufferSize is the capacity of the scheduler's stream queue.
	DefaultBufferSize uint = 200

	// DefaultEnqueueTimeout bounds how long a submit waits for space in a full
	// stream queue before the request is rejected. A saturated scheduler then
	// sheds load instead of parking handler goroutines without bound.
	DefaultEnqueueTimeout time.Duration = 2 * time.Second

	// DefaultLogLevel is the minimum log level emitted. It names a constant
	// from validate.go instead of repeating a literal, so the default cannot
	// drift from the values validate accepts.
	DefaultLogLevel string = LogLevelInfo

	// DefaultLogFormat is the log output format.
	DefaultLogFormat string = LogFormatText

	// DefaultLogDisableTimestamp keeps timestamps on. A log read from a file
	// or a terminal has no other clock. A supervisor that stamps its own lines
	// is the case that opts out.
	DefaultLogDisableTimestamp bool = false

	// DefaultHashingFunction is the hash used to derive node keys from values.
	DefaultHashingFunction string = HashingXxhash

	// DefaultHashingFunctionSeed seeds the node-key hashing function.
	DefaultHashingFunctionSeed uint64 = 0

	// DefaultSearchAlgorithm is the traversal that moves seed evidence through
	// the graph. Excess transmission is the shipped methodology. "bfs" is kept
	// for comparison runs. "none" turns the graph channel off, leaving text and
	// vector search only.
	DefaultSearchAlgorithm string = SearchExcess

	// DefaultScoringAlgorithm selects scoring.InformationScorer, the shipped
	// scoring methodology: the text, vector and graph channels weighed by the
	// bits each carries about every candidate. Measured on the ten LoCoMo
	// conversations, one graph each, with text-embedding-3-small vectors: it
	// beats "excess" on precision, recall and F1 at every k, with recall@10
	// 0.911 against 0.909, multi-hop 0.728 against 0.699, temporal 0.920
	// against 0.897 and open-domain 0.677 against 0.658. It does not keep the
	// BM25 floor: weighing the graph against the text, it can rank a fact the
	// graph raised above one the words matched better. "excess" selects
	// scoring.ExcessScorer, which keeps it, and "rrf" scoring.RRFScorer, kept
	// for comparison runs.
	DefaultScoringAlgorithm string = ScoringInformation

	// DefaultRelevanceModel is the text index's relevance model. The excess
	// methodology needs BM25's raw retrieval mass. "matchcount" is kept for
	// comparison runs.
	DefaultRelevanceModel string = RelevanceBM25

	// DefaultTop is how many ranked results a recall returns when no top clause
	// is given.
	DefaultTop int = 10

	// DefaultDepth is the depth a recall uses when no depth clause is given.
	// It is 0, the floor lane, which ranks by seed mass alone and skips the
	// anchor traversal. A recall that names a topic or entity opts into the
	// traversal with a higher depth:
	//   - depth:1 is the precision lane. Only strongly above-chance anchors
	//     transmit.
	//   - depth:2 is max recall. Any anchor above its fair share transmits.
	DefaultDepth int = 0

	// DefaultMaxTop is the ceiling on a recall's top clause. A request for
	// more results is rejected at parse time, so one query cannot force an
	// unbounded result set.
	DefaultMaxTop int = 1000

	// DefaultMaxDepth is the ceiling on a recall's depth clause. The three
	// lanes are depth 0 (floor), depth 1 (precision) and depth 2 (max recall).
	// Search runs a single anchor-mediated round, so a deeper request would
	// behave exactly like depth 2. It is rejected at parse time instead.
	DefaultMaxDepth int = 2

	// DefaultMaxVectorDimension is the ceiling on the length of a bound vector
	// parameter. A longer vector is rejected at parse time. This bounds the
	// work an index insert or search can be asked to do.
	DefaultMaxVectorDimension int = 4096

	// DefaultHalflife is the time-decay half-life applied to fact scores. Decay
	// multiplies the whole score, so the half-life bounds how far recency
	// overrides relevance. A long-term memory measures it in months. The newer
	// of two equal facts still ranks first. A strong match from last quarter
	// still outranks a weak one from today.
	DefaultHalflife time.Duration = 90 * 24 * time.Hour

	// DefaultSeedSize is the minimum candidate budget each source (text and
	// vector index) contributes to a search. The effective budget is
	// max(seed-size, top), so a large recall is never starved of candidates.
	DefaultSeedSize int = 10

	// DefaultAggregatePool is how many ranked candidates a recall's slots are
	// spread from.
	DefaultAggregatePool int = 60

	// DefaultAggregateCap is how many fact hits one facet takes before the
	// next facet gets its first. It is 1, the recall setting, so the slots
	// cover as many facets as the pool offers. Measured on the ten LoCoMo
	// conversations, one graph each, against the ranking taken as it is:
	//   - session recall@10: 0.932 against 0.908
	//   - multi-hop: 0.757 against 0.679
	//   - open-domain: 0.712 against 0.664
	//   - precision: 0.117 against 0.177
	DefaultAggregateCap int = 1

	// DefaultCacheCapacity is the size of the LRU cache of optimised query plans.
	DefaultCacheCapacity int = 1000

	// DefaultProjectionDimension is the dimension vectors are randomly projected
	// down to inside each RP-tree. It is the number of split directions a tree
	// can draw on. With a narrow projection, every level of a deep tree reuses
	// the same few directions, and the partition stops resembling the space.
	// Ingest does not pay for a wide one. A split names one row, and routing
	// reads one row (Projection.ApplyRow).
	DefaultProjectionDimension int = 128

	// DefaultNumberTrees is how many RP-trees form the vector index forest. A
	// single tree is a weak approximator. The forest makes up for it with
	// independent projections, so vector recall depends on the tree count more
	// than on any other setting. Measured on 50k 128-d vectors, going from 4 to
	// 16 trees roughly quadruples recall@10 at every projection dimension.
	// Query cost, write cost and memory grow linearly with it. The default
	// favours recall over latency.
	DefaultNumberTrees int = 16

	// DefaultRPSeed seeds the RP-trees' random projections (deterministic builds).
	DefaultRPSeed uint64 = 4

	// DefaultLeafSize is how many points an RP-tree leaf holds before it splits
	// into two. It sets the granularity of the partition, and so the floor on
	// what one probe examines. Larger leaves mean fewer, coarser regions and
	// more candidates scanned per probe. Smaller leaves mean the reverse. It is
	// the least useful vector setting to move, because overfetch reaches the
	// same candidate counts without rebuilding the index.
	DefaultLeafSize int = 32

	// DefaultOverfetch is how many candidates a vector search gathers per result
	// asked for before it stops probing. A pool of exactly the requested size
	// leaves the true-distance re-ranking nothing to choose from. Overfetch is
	// what turns probing into recall, and at the limit it becomes an exact
	// scan. It shapes no index, so it costs query time only, not write cost or
	// resident memory. Recall per unit of query time stays flat as it rises,
	// so it is a budget choice rather than an optimum. Measured on 50k uniform
	// 128-d vectors, recall@10 is 0.12 at 8, 0.24 at 16, 0.42 at 32 and 0.53
	// at 64.
	DefaultOverfetch int = 32

	// DefaultFlushFactor bounds RP forest garbage. Once a tree holds more than
	// this many entries per live vector, the forest is rebuilt from the live set.
	DefaultFlushFactor int = 2

	// DefaultPrecision is the floating-point precision for embeddings and scores,
	// "float32" or "float64". It selects which server instantiation is built.
	DefaultPrecision string = PrecisionFloat32

	// DefaultMCPAddressFormat is the address the mcp bridge forwards to when
	// none is configured. It is the local daemon, on whatever port the same
	// config gives it.
	DefaultMCPAddressFormat string = "http://127.0.0.1:%d"
)
