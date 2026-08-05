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

## Scheduler

The `Scheduler` manages when and how streams are executed. It implements a work-queue pattern:

```
Submit(stream)
     ↓
Queue (buffered channel)
     ↓
Workers (pool of goroutines)
     ↓
execute(stream) → Acquire lock → Commit in place
```

### Configuration

The scheduler is configured via `config.Scheduler`:

- **BufferSize**: Capacity of the work queue (default varies by config)
- **Workers**: Number of concurrent worker goroutines (configurable)

Typical configuration:

```go
Config.Scheduler.BufferSize = 1000   // queue depth
Config.Scheduler.Workers = 4          // parallel workers
```

### Worker Lifecycle

When the scheduler starts:

1. A buffered channel of size `BufferSize` is created
2. `Workers` goroutines are spawned, each waiting on the channel
3. Streams submitted via `Submit()` are placed on the queue
4. Workers process streams in queue order (FIFO)

When the scheduler stops:

1. The queue channel is closed
2. Remaining workers drain the queue and exit
3. All goroutines are waited on (via `sync.WaitGroup`)

### Stream Execution

Each worker processes a single stream at a time:

```go
func (s *Scheduler) worker() {
    for stream := range s.Queue {
        err := s.execute(stream)
        if err != nil {
            logger.Error("Failed to execute stream", "error:", err)
        }
    }
}
```

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
