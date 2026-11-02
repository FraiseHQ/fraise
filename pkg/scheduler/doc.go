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

// Package scheduler runs planned query streams against the database: reads on
// a bounded worker pool, writes on one writer per graph.
//
// [Scheduler.Submit] enqueues a stream, a read on the shared read queue and a
// write on its graph's lane. It never blocks without bound, so the server can
// shed load. It fails with [ErrQueueFull] when that queue stays full past the
// enqueue timeout, and with [ErrShutdown] once the scheduler is stopping. Each
// stream runs under its graph's lock: shared for a read, exclusive for a
// write. So writes to one graph are serialised, in the order their lane
// accepted them, while reads run together. Keeping the two apart means a read
// worker never parks on a write lock and a write never waits for a read
// worker. [Scheduler.Stop] drains every stream already accepted before it
// returns, so a graceful shutdown never drops an acknowledged write.
package scheduler
