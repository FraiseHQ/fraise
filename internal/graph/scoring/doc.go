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

// Package scoring folds the observations a search collects for a candidate
// into its relevance score.
//
// Collection records each observation as a [Contribution], tagged with the
// [Source] that made it (text, vector, graph traversal or anchor seeding) and
// carrying that source's raw mass, rank and anchor statistics, with no policy
// applied. A [Scorer] owns the policy: [ExcessScorer], the default, sums the
// seed mass and adds each anchor's attenuated surplus over its fair share
// under the query's background rate, while [RRFScorer] fuses ranks alone for
// comparison runs. Keeping policy out of collection means changing how
// ranking works never touches the retrieval sites.
//
// One Scorer instance is shared by every concurrent search on a graph, so a
// Scorer is pure, and the background rate is bound per query through
// [Scorer.WithBackground] rather than written into the shared instance.
package scoring
