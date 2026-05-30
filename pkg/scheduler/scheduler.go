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
	"fmt"

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

// The scheduler decides when to run a stream.
// one concurrent write stream is supported at a time
// The scheduler decides when to wait for a write operation to finish
type Scheduler[K comparable, P float32 | float64] struct {
	Graph         *graph.KnowledgeGraph[K, P]
	Config        *config.ConfigSet
	writeInFlight bool
	Queue         containers.Queue[*Stream[K, P]]
}

func NewScheduler[K comparable, P float32 | float64](config *config.ConfigSet) (*Scheduler[K, P], error) {
	return nil, nil
}

func (s *Scheduler[K, P]) Start() error {
	return nil
}

func (s *Scheduler[K, P]) Stop() error {
	return nil
}

func (s *Scheduler[K, P]) Next() *Stream[K, P] {
	return &Stream[K, P]{}
}

func (s *Scheduler[K, P]) Submit(tx *Stream[K, P]) error {
	err := s.Queue.Push(tx)

	if err != nil {
		return fmt.Errorf("")
	}
	return nil
}

func (s *Scheduler[K, P]) Execute(tx *Stream[K, P]) error {
	return nil
}
