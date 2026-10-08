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

// Package scheduler runs planned query streams against the database on a
// bounded worker pool.
//
// [Scheduler.Submit] enqueues a stream and fails fast rather than blocking
// without bound: [ErrQueueFull] when the queue stays saturated past the
// enqueue timeout, [ErrShutdown] once the scheduler is stopping, so the
// server can shed load. A worker runs each stream under its graph's lock,
// shared for a read and exclusive for a write, so writes to one graph are
// serialised while reads proceed together. [Scheduler.Stop] drains every
// stream already accepted before returning, so a graceful shutdown never
// drops an acknowledged write.
package scheduler
