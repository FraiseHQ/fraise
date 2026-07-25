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
	DefaultConfigFile = "fraise.config.toml"

	DefaultPort = 9876

	DefaultNumGraph uint8 = 8

	DefaultWorkersCount int = 2

	DefaultBufferSize uint = 200

	DefaultLogLevel string = "INFO"

	DefaultLogFormat string = "text"

	DefaultHashingFunction string = "xxhash"

	DefaultHashingFunctionSeed uint64 = 0

	DefaultSearchAlgorithm string = "none"

	DefaultRankingAlgorithm string = "none"

	DefaultPageRankDamping float64 = 0.85

	DefaultPageRankMaxIter int = 100

	DefaultPageRankTol float64 = 1e-6

	DefaultTop int = 10

	DefaultDepth int = 2

	DefaultAllowUnanchoredRecall bool = false

	DefaultHalflife time.Duration = 7 * 24 * time.Hour

	DefaultSeedSize uint = 10

	DefaultHopAttenuation float32 = 0.5

	DefaultCacheCapacity int = 1000

	DefaultProjectionDimention int = 8

	DefaultNumberTrees int = 4

	DefaultRPSeed int = 4
)
