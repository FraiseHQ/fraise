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
	// request (headers + body). It stops a slow client from pinning a worker.
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

	// DefaultMaxBodyBytes caps the size of a request body the query endpoint will
	// read. A larger body is rejected before it is buffered, bounding memory.
	DefaultMaxBodyBytes int64 = 1 << 20 // 1 MiB

	// DefaultNumGraph is how many independent graphs the store allocates; valid
	// selectors are 0..DefaultNumGraph-1. Graph selectors are uint8, so values
	// above 256 leave the extra graphs unreachable.
	DefaultNumGraph int = 8

	// DefaultWorkersCount is the number of scheduler worker goroutines that
	// execute query streams.
	DefaultWorkersCount int = 2

	// DefaultBufferSize is the capacity of the scheduler's stream queue.
	DefaultBufferSize uint = 200

	// DefaultEnqueueTimeout bounds how long a submit waits for space in a full
	// stream queue before the request is rejected, so a saturated scheduler
	// sheds load instead of parking handler goroutines without bound.
	DefaultEnqueueTimeout time.Duration = 2 * time.Second

	// DefaultLogLevel is the minimum log level emitted. Named from the accepted
	// spellings in validate.go rather than repeated as a literal: a default that
	// is not itself an accepted value is the drift that left "text" out of the
	// logger's switch.
	DefaultLogLevel string = LogLevelInfo

	// DefaultLogFormat is the log output format.
	DefaultLogFormat string = LogFormatText

	// DefaultHashingFunction is the hash used to derive node keys from values.
	DefaultHashingFunction string = HashingXxhash

	// DefaultHashingFunctionSeed seeds the node-key hashing function.
	DefaultHashingFunctionSeed uint64 = 0

	// DefaultSearchAlgorithm is the graph traversal used to expand seeds; "none"
	// falls back to the built-in breadth-first walk.
	DefaultSearchAlgorithm string = SearchNone

	// DefaultRankingAlgorithm is the global ranking boost applied to walk scores;
	// "none" disables it (the alternative is "pagerank").
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

	// DefaultDepth is how many hops a recall walk leaves the seed when no depth
	// clause is given.
	DefaultDepth int = 2

	// DefaultMaxTop is the ceiling on a recall's top clause. A request asking for
	// more than this many results is rejected at parse time, so a single query
	// cannot force an unbounded result set.
	DefaultMaxTop int = 1000

	// DefaultMaxDepth is the ceiling on a recall's depth clause. Walk cost grows
	// with depth, so a request past this many hops is rejected at parse time.
	DefaultMaxDepth int = 6

	// DefaultMaxVectorDimension is the ceiling on the length of a bound vector
	// parameter. A longer vector is rejected at parse time, bounding the work an
	// index insert or search can be asked to do.
	DefaultMaxVectorDimension int = 4096

	// DefaultAllowUnanchoredRecall controls whether a recall with no anchor
	// (entity/topic) is permitted.
	DefaultAllowUnanchoredRecall bool = false

	// DefaultHalflife is the time-decay half-life applied to fact scores.
	DefaultHalflife time.Duration = 7 * 24 * time.Hour

	// DefaultSeedSize is how many seeds each source (text and vector index)
	// contributes to a search.
	DefaultSeedSize uint = 10

	// DefaultCacheCapacity is the size of the LRU cache of optimised query plans.
	DefaultCacheCapacity int = 1000

	// DefaultProjectionDimention is the dimension vectors are randomly projected
	// down to inside each RP-tree.
	DefaultProjectionDimention int = 8

	// DefaultNumberTrees is how many RP-trees form the vector index forest.
	DefaultNumberTrees int = 4

	// DefaultRPSeed seeds the RP-trees' random projections (deterministic builds).
	DefaultRPSeed uint64 = 4

	// DefaultFlushFactor bounds RP forest garbage: once a tree holds more than
	// this many entries per live vector, the forest is rebuilt from the live set.
	DefaultFlushFactor int = 2

	// DefaultPrecision is the floating-point precision for embeddings and scores
	// ("float32" or "float64"), selecting which server instantiation is built.
	DefaultPrecision string = PrecisionFloat32
)
