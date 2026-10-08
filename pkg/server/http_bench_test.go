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

package server_test

import (
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	vegeta "github.com/tsenart/vegeta/v12/lib"
)

const (
	// httpRate and httpDuration fix the load at a constant arrival rate, so a
	// slow response delays no later request and the percentiles keep the
	// queueing a real client would see.
	httpRate     = 200
	httpDuration = 20 * time.Second

	// httpMix is how many requests the load cycles through, one write in
	// every httpWriteEvery.
	httpMix        = 1000
	httpWriteEvery = 10
)

// httpFact is one deterministic fact: twelve words drawn from a Zipf
// distribution, so a few terms carry long posting lists as in real text, filed
// under one of fifty topics and one of five hundred entities.
func httpFact(rng *rand.Rand, zipf *rand.Zipf) string {
	words := make([]string, 12)
	for w := range words {
		words[w] = "w" + strconv.FormatUint(zipf.Uint64(), 10)
	}
	return fmt.Sprintf("remember@0 '%s' topic:t%d entity:e%d", strings.Join(words, " "), rng.Intn(50), rng.Intn(500))
}

// httpTargets is the request mix the load cycles through: recalls of two or
// three corpus words, scoped to a topic and spread over the three depths, and
// one new fact in every httpWriteEvery requests. The same seed yields the
// same requests in the same order on every run.
func httpTargets(b *testing.B, url string, rng *rand.Rand, zipf *rand.Zipf) []vegeta.Target {
	header := http.Header{"Content-Type": []string{"application/json"}}
	targets := make([]vegeta.Target, httpMix)
	for i := range targets {
		var fql string
		if i%httpWriteEvery == 0 {
			fql = httpFact(rng, zipf)
		} else {
			terms := make([]string, 2+rng.Intn(2))
			for t := range terms {
				terms[t] = "w" + strconv.FormatUint(zipf.Uint64(), 10)
			}
			fql = fmt.Sprintf("recall@0 %s topic:t%d top:10 depth:%d", strings.Join(terms, " "), rng.Intn(50), i%3)
		}
		targets[i] = vegeta.Target{Method: http.MethodPost, URL: url + "/api/v1/q", Body: requestBody(b, fql, nil), Header: header}
	}
	return targets
}

// BenchmarkHTTP measures request latency through the whole server, HTTP and
// scheduler included, at ten and a hundred thousand facts: a fresh server is
// filled with a deterministic corpus, then held at a constant rate of mixed
// recalls and writes, and the p50, p95 and p99 of the responses are reported
// in nanoseconds. The attack's duration, not b.N, sets how much it measures,
// so run it with -benchtime 1x.
func BenchmarkHTTP(b *testing.B) {
	for _, facts := range []int{10_000, 100_000} {
		b.Run(fmt.Sprintf("facts-%d", facts), func(b *testing.B) {
			url := startServer(b)
			rng := rand.New(rand.NewSource(1))
			zipf := rand.NewZipf(rng, 1.1, 1, 5000)
			for range facts {
				query(b, url, httpFact(rng, zipf), nil)
			}
			targeter := vegeta.NewStaticTargeter(httpTargets(b, url, rng, zipf)...)
			attacker := vegeta.NewAttacker(vegeta.KeepAlive(true))

			var m vegeta.Metrics
			for b.Loop() {
				m = vegeta.Metrics{}
				for res := range attacker.Attack(targeter, vegeta.Rate{Freq: httpRate, Per: time.Second}, httpDuration, b.Name()) {
					m.Add(res)
				}
				m.Close()
			}
			if m.Success < 1 {
				b.Fatalf("%.2f%% of requests failed: status codes %v, errors %v", 100*(1-m.Success), m.StatusCodes, m.Errors)
			}
			b.ReportMetric(float64(m.Latencies.P50.Nanoseconds()), "p50-ns")
			b.ReportMetric(float64(m.Latencies.P95.Nanoseconds()), "p95-ns")
			b.ReportMetric(float64(m.Latencies.P99.Nanoseconds()), "p99-ns")
		})
	}
}
