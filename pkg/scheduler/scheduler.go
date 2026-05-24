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

package scheduler

import (
	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/internal/containers"
	"github.com/RonsenbergVI/fraise/internal/graph"
)

type Task struct {
	Command string
	Args    string
	Result  chan TaskResult
}

type TaskResult struct {
	Value []byte
	Err   error
}

// The scheduler decides when to run a transaction.
// one concurrent write transaction is supported at a time
// The scheduler decides when to wait for a write operation to finish
type Scheduler[K comparable, V string | ~float32 | ~int | ~bool, P float32 | float64] struct {
	Graph         *graph.KnowledgeGraph[K, V, P]
	Config        *config.ConfigSet
	writeInFlight bool
	Queue         containers.Queue[*Transaction[K, V, P]]
}

func (s *Scheduler[K, V, P]) Start() error {
	return nil
}

func (s *Scheduler[K, V, P]) Stop() error {
	return nil
}

func (s *Scheduler[K, V, P]) Next() *Transaction[K, V, P] {
	return &Transaction[K, V, P]{}
}

func (s *Scheduler[K, V, P]) Submit(tx *Transaction[K, V, P]) error {
	return nil
}

func (s *Scheduler[K, V, P]) Execute(tx *Transaction[K, V, P]) error {
	return nil
}
