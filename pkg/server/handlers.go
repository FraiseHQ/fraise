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

package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/FraiseHQ/fraise/internal/index"
	"github.com/FraiseHQ/fraise/internal/query"
	"github.com/FraiseHQ/fraise/internal/query/parser"
	"github.com/FraiseHQ/fraise/pkg/logger"
	"github.com/FraiseHQ/fraise/pkg/scheduler"
	"github.com/FraiseHQ/fraise/pkg/version"
	"github.com/gin-gonic/gin"
)

// errorToResponse maps an error from the query pipeline to an HTTP status code
// and a client-safe message. It is the one place that decides how a parse, plan
// or execution error is surfaced:
//
//   - A *parser.Error is a client mistake (400). Its position is safe to show.
//   - The known client sentinels are 400: a bad parse, a missing parameter, a
//     limit exceeded, and a vector whose dimension does not match its graph.
//   - Anything else is an internal fault (500) with a generic message, so
//     internal detail never reaches a client. Callers log the error.
func errorToResponse(err error) (int, string) {
	var perr *parser.Error
	switch {
	case errors.As(err, &perr):
		return http.StatusBadRequest, perr.Error()
	case errors.Is(err, query.ErrParsingFailed),
		errors.Is(err, query.ErrMissingParameter),
		errors.Is(err, query.ErrLimitExceeded),
		errors.Is(err, index.ErrInvalidDimension):
		return http.StatusBadRequest, err.Error()
	default:
		return http.StatusInternalServerError, "internal server error"
	}
}

// handleHealthCheck returns a handler that reports the server is alive. It
// answers 200 with a status and the running version.
//
// The version is the SDK's only handshake. The SDK reads it to check that the
// server falls inside the range it supports (see COMPATIBILITY.md). So the key
// must not be renamed or dropped without a matching SDK release.
func (s *Server[K, P]) handleHealthCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"version": version.FullVersion(),
		})
	}
}

// handleStats returns a handler that snapshots every graph's shape: nodes,
// edges, vectors and forest entries. It makes internal invariants observable
// for monitoring and end-to-end tests. For example, the vector forest must stay
// O(live vectors) under sustained writes.
func (s *Server[K, P]) handleStats() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, s.DB.Stats())
	}
}

// handleQuery returns a handler that binds the JSON request body and parses the
// query. The engine plans and schedules it, and the handler answers once the
// stream has run.
//
// explain selects the explain mode served on /api/v1/explain. It runs the same
// pipeline, but each hit carries its per-source contribution breakdown. Writes
// are rejected, since only a recall has a ranking to explain. One handler with
// a flag, rather than two handlers, keeps the two routes from drifting apart in
// how they parse, validate or schedule.
func (s *Server[K, P]) handleQuery(explain bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req HandleQueryRequest[P]
		if err := c.ShouldBindJSON(&req); err != nil {
			logger.Warn("Rejecting malformed query request body", "error", err)
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}

		logger.Debug("Query received", "query", req.Query, "parameters", len(req.Parameters))

		// Parse binds any vector placeholder (vec:$v) from the request
		// parameters. Warnings flag a query that runs anyway, such as one with
		// a stop word among its terms. They are returned with the results.
		q, warns, err := query.Parse[K, P](req.Query, req.Parameters, s.Config)

		if err != nil {
			logger.Warn("Rejecting unparsable query", "query", req.Query, "error", err)
			status, msg := errorToResponse(err)
			c.JSON(status, ErrorResponse{Error: msg})
			return
		}

		// Only reads have a ranking to explain. Rejecting a remember here keeps
		// explain free of side effects, so probing a ranking can never mutate a
		// graph.
		if explain && q.IsWrite() {
			logger.Warn("Rejecting write query on explain endpoint", "query", req.Query)
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: "explain applies to recall queries only",
			})
			return
		}

		// A selector outside the allocated range is a client error. Reject it
		// here rather than let it reach the scheduler and fail deep in Select.
		if int(q.GetGraphID()) >= s.DB.NumGraphs() {
			logger.Warn("Rejecting out-of-range graph selector",
				"graph", q.GetGraphID(), "max", s.DB.NumGraphs()-1)
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: fmt.Sprintf("graph %d does not exist (valid range 0-%d)",
					q.GetGraphID(), s.DB.NumGraphs()-1),
			})
			return
		}

		stream, err := s.Engine.Plan(q)

		if err != nil {
			logger.Error("Failed to plan query", "query", req.Query, "error", err)
			status, msg := errorToResponse(err)
			c.JSON(status, ErrorResponse{Error: msg})
			return
		}

		// Explain belongs to this execution, so it is set on the stream after
		// planning. The plan cache substitutes query objects on a hash hit, so
		// a flag on the query would leak between requests.
		stream.Explain = explain

		logger.Debug("Query planned, dispatching to engine",
			"graph", q.GetGraphID(), "write", q.IsWrite())

		// A stream that cannot be enqueued never runs, so the handler must not
		// wait on its completion.
		if err := s.Engine.Apply(c.Request.Context(), stream); err != nil {
			if errors.Is(err, scheduler.ErrShutdown) {
				logger.Warn("Rejecting query, server shutting down", "query", req.Query)
				c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "server shutting down"})
				return
			}
			// The queue stayed full past the enqueue timeout. Shed load with a
			// 429 so clients back off instead of piling up blocked requests.
			if errors.Is(err, scheduler.ErrQueueFull) {
				logger.Warn("Rejecting query, scheduler queue full", "query", req.Query)
				c.JSON(http.StatusTooManyRequests, ErrorResponse{Error: "server overloaded, retry later"})
				return
			}
			// The request context was cancelled before the stream was enqueued,
			// because the client disconnected. No live connection is left to
			// answer.
			logger.Warn("Query not enqueued", "query", req.Query, "error", err)
			return
		}

		select {
		case <-stream.Done():
			if stream.Err != nil {
				logger.Error("Query execution failed", "query", req.Query, "error", stream.Err)
				status, msg := errorToResponse(stream.Err)
				c.JSON(status, ErrorResponse{Error: msg})
				return
			}
			logger.Info("Query executed", "query", req.Query, "graph", q.GetGraphID())

			// A successful write or read carries the warnings, so they are
			// rendered once here for whichever branch below answers.
			msgs := make([]string, len(warns))
			for i, w := range warns {
				msgs[i] = w.String()
			}

			// A write answers in its own shape, not a read's envelope.
			// WriteResponse says why the two must not be the same bytes.
			if q.IsWrite() {
				c.JSON(http.StatusOK, WriteResponse{Status: writeStatusOK, Warnings: msgs})
				return
			}

			// A read of an empty graph is a different answer from a miss on a
			// populated one. The first says the caller has yet to write. The
			// second says the query missed. A 204 carries that difference in
			// the status line. It has no body, so warnings are dropped. They
			// are moot when there was nothing to search.

			if stream.Result.Count == 0 && stream.IsGraphEmpty {
				c.Status(http.StatusNoContent)
				return
			}

			// The warnings key appears only when there is something to say, so
			// a clean query keeps the plain response shape.
			resp := gin.H{"results": stream.Result}
			if len(msgs) > 0 {
				resp["warnings"] = msgs
			}
			c.JSON(http.StatusOK, resp)
		case <-c.Request.Context().Done():
			logger.Warn("Client disconnected before query completed", "query", req.Query)
			return
		}
	}
}
