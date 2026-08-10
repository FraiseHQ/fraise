# Concurrency in Fraise

This document explains how Fraise handles concurrent operations through its multi-graph architecture, scheduler, and fine-grained locking strategy.

## Overview

Fraise employs three key mechanisms to handle concurrency:

1. **Multi-graph sharding**: Distributing data across independent in-memory graphs
2. **Worker pool scheduler**: Processing streams through configurable workers
3. **Per-graph read-write locks**: Protecting individual graph state

Together, these enable high-performance concurrent reads with serialized writes per graph.

## Multi-Graph Architecture

The database maintains multiple independent temporal memory graphs by default (8 graphs). Each graph is a completely separate in-memory data structure containing:

- Nodes (entities, facts, relationships)
- Text search indices
- Vector (semantic) indices
- Adjacency maps for graph traversal

### Graph Partitioning

Streams specify a target graph via `Query.GetGraphID()`, which returns an index (0-7 by default). This partitioning allows:

- Multiple writes to **different graphs** to proceed in parallel
- Reads from any graph at any time (unless a write is in progress on that graph)
- Independent scaling: no global lock contention

### When to Use Multiple Graphs

Partition your data across graphs when:

- You have multiple agents with separate memory contexts
- Data is naturally independent (e.g., per-user memory, per-conversation state)
- You want to reduce lock contention by distributing writes

All graphs within a database share the same underlying configuration and lifecycle (Start/Stop).

A graph is also an isolation boundary, not only a lock domain: no query reads
across graphs, and no walk traverses between them. That is why the graph
selector is validated twice — the parser rejects a selector that would not fit
`uint8` (unchecked, `@256` would wrap to graph 0 and read another tenant's
memory), and the server rejects one past the allocated count. Concurrency is the
reason to have several graphs; isolation is the reason to be careful which one a
query names.

## Scheduler

The `Scheduler` manages when and how streams are executed. It implements a work-queue pattern:

```text
Submit(stream)
     ↓
Queue (buffered channel)
     ↓
Workers (pool of goroutines)
     ↓
execute(stream) → Acquire lock → Commit in place
```

### Configuration

The scheduler is configured under `[scheduler]` (see
[`configuration.md`](configuration.md)):

- **workers**: number of concurrent worker goroutines (default 2)
- **buffer-size**: capacity of the work queue (default 200)
- **enqueue-timeout**: how long `Submit` waits for room before shedding
  (default 2s)

The three are one setting in three parts: `workers` bounds how much executes at
once, `buffer-size` bounds how much waits, and `enqueue-timeout` bounds how long
a caller is willing to wait to become one of the waiting. Raising the first two
without the third just moves where requests pile up.

### Submitting Work

`Submit` is bounded and context-aware, which is what turns overload into
backpressure instead of an ever-growing pile of blocked goroutines. It returns:

- `nil` once the stream is on the queue
- `ErrQueueFull` if the queue stayed saturated for `enqueue-timeout` — the
  server answers **429**, telling the client to back off
- `ErrShutdown` if the scheduler is stopping or was never started — **503**
- a wrapped `ctx.Err()` if the caller gave up first, in which case there is no
  live request left to answer

A caller is therefore never parked indefinitely on a full, nil, or closed queue.

### Worker Lifecycle

When the scheduler starts:

1. A buffered channel of size `buffer-size` is created, alongside a `quit`
   channel
2. `workers` goroutines are spawned, each selecting on both
3. Streams submitted via `Submit()` are placed on the queue
4. Workers process streams in queue order (FIFO)

When the scheduler stops:

1. `quit` is closed — **the queue itself is never closed**. Closing it would
   panic a `Submit` that is mid-send; closing a separate channel refuses new
   work without that race, and unparks any `Submit` waiting on a full queue.
2. Each worker drains what is already buffered, then exits
3. All goroutines are waited on (via `sync.WaitGroup`)
4. `Stop` drains any straggler a `Submit` raced into the buffer as `quit`
   closed. This runs single-threaded once the workers are gone, so an accepted
   write is executed rather than silently dropped

The invariant across all four steps: work that `Submit` accepted is work that
runs. A write acknowledged and then dropped by shutdown would be indistinguishable
from data loss.

### Stream Execution

Each worker processes a single stream at a time, taking whichever of queue or
quit is ready:

```go
func (s *Scheduler[K, P]) worker(queue chan *query.Stream[K, P], quit chan struct{}) {
    defer s.wg.Done()
    for {
        select {
        case stream := <-queue:
            if err := s.execute(stream); err != nil {
                logger.Error("Failed to execute stream", "error", err)
            }
        case <-quit:
            // drain what is already buffered, then return
            return
        }
    }
}
```

Whatever happens, `execute` signals completion: `stream.Finish()` is deferred
before anything that can fail, so a request waiting on `Done()` is never left
hanging by an early error such as an out-of-range graph selector.

The `execute()` method:

1. **Selects the graph**: `DB.Select(stream.Query.GetGraphID())`
2. **Acquires the graph's lock**: `stream.Acquire(g)` — the write lock for
   writes, a read lock for reads
3. **Commits in place**: `stream.Commit(g)` — reads run the search against the
   live graph; writes mutate it directly under the exclusive lock, costing
   O(fact + incremental index updates) regardless of graph size. The lock is
   already exclusive, so no staging copy is needed: nothing can observe
   intermediate state. On error the stream records it and completes; the one
   realistic write failure (vector-dimension mismatch) is checked before any
   mutation, leaving the graph untouched.

## Locking Strategy

### Per-Graph Locks

Each graph holds a `sync.RWMutex` protecting its internal state:

```go
type InMemoryGraph[K comparable, P float32 | float64] struct {
    idToNodes     map[K]Node[K]
    nodeToSources map[K]map[K]Relationship[K]
    nodeToTargets map[K]map[K]Relationship[K]
    mu            sync.RWMutex
}
```

Public methods for lock management:

- `RLock()` / `RUnlock()` — acquire/release a read lock
- `Lock()` / `Unlock()` — acquire/release a write lock

### Lock Holder Responsibility

The graph interface exposes locks to allow callers to compose operations while holding a single lock. For example:

```go
g.Lock()
defer g.Unlock()

node := g.Get(key)    // read under lock
if node != nil {
    g.Put(key, modified)  // write under lock
}
```

This prevents **read-modify-write races**. Without explicit composition, the graph operations would need internal locking (not currently implemented in the interface contract).

### Read-Write Semantics

- **Reads**: Multiple goroutines can read from a graph simultaneously (`RLock()`)
- **Writes**: Exclusive access; blocks all readers and other writers (`Lock()`)
- **No upgrade**: A read lock cannot be upgraded to a write lock without releasing first

## Concurrency Guarantees

### Per-Graph Atomicity

All operations on a single graph are serialized through its lock:

- Concurrent writes to the same graph **will serialize**
- Concurrent reads do not block each other
- Writes block concurrent reads

### Cross-Graph Independence

Operations on different graphs are fully independent:

- 4 workers can write to 4 different graphs in parallel
- No global coordination or lock contention

### Stream Ordering Within a Graph

Streams submitted to the scheduler are processed **in FIFO order** (per worker). However, because there are multiple workers:

- Streams targeting the same graph may be processed by different workers
- Lock ordering is still FIFO (whoever acquires the lock first goes first)
- There is no guaranteed ordering across multiple workers

If strict ordering is required, submit streams to a single worker or use a dedicated queue.

## Practical Implications

### High Concurrency Scenario

```
4 workers, 8 graphs
↓
Agent 1 submits write to graph 0
Agent 2 submits write to graph 1
Agent 3 submits write to graph 2
Agent 4 submits write to graph 3
↓
All 4 writes proceed in parallel (different graphs)
```

### Contention Scenario

```
4 workers, 8 graphs
↓
Agent 1 submits write to graph 0
Agent 2 submits write to graph 0
Agent 3 submits write to graph 0
Agent 4 submits write to graph 0
↓
Writes serialize on graph 0's lock
```

### Avoiding Lock Contention

1. **Distribute data**: Use multiple graphs; partition by agent, conversation, or domain
2. **Batch reads**: Use `RLock()` to read multiple entities in one critical section
3. **Minimize lock hold time**: writes commit in place, so lock hold time is
   proportional to the fact being written — keep facts small rather than
   batching many into one stream
4. **Monitor queue depth**: If the queue fills up, consider increasing `BufferSize` or `Workers`

## Future Considerations

- **Write ordering**: Provide a guarantee or API to enforce stream ordering across graphs
- **Lock profiling**: Add metrics for lock contention and wait times
- **Optimistic locking**: Consider versioning for read-optimized workloads
- **Lock-free data structures**: Explore alternatives like `sync.Map` for specific use cases
