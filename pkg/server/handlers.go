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

	"github.com/RonsenbergVI/fraise/internal/query"
	"github.com/RonsenbergVI/fraise/internal/query/parser"
	"github.com/RonsenbergVI/fraise/pkg/logger"
	"github.com/gin-gonic/gin"
)

// errorToResponse maps an error coming out of the query pipeline to an HTTP
// status code and a client-safe message. It is the single place that decides
// how an internal error is surfaced, so handlers never hand-pick status codes:
//
//   - a *parser.Error is a client mistake (400) and its position is safe to show;
//   - known client sentinels (bad parse, missing parameter) are 400;
//   - anything else is treated as an internal fault (500) with a generic
//     message, so we never leak internal error detail to clients (we log it).
func errorToResponse(err error) (int, string) {
	var perr *parser.Error
	switch {
	case errors.As(err, &perr):
		return http.StatusBadRequest, perr.Error()
	case errors.Is(err, query.ErrParsingFailed),
		errors.Is(err, query.ErrMissingParameter):
		return http.StatusBadRequest, err.Error()
	default:
		return http.StatusInternalServerError, "internal server error"
	}
}

// handleHealthCheck returns a handler that reports the server is alive,
// responding with HTTP 200 and a simple status payload.
func (s *Server[K, P]) handleHealthCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

// handleQuery returns a handler that parses, plans, and executes a query.
// It binds the JSON request body, parses the query string, asks the engine
// for an execution plan, applies it, and streams back the results. Any
// failure along the way is reported as an HTTP error.
func (s *Server[K, P]) handleQuery() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Decode the request body; reject malformed JSON with 400.
		var req HandleQueryRequest[P]
		if err := c.ShouldBindJSON(&req); err != nil {
			logger.Warn("Rejecting malformed query request body", "error", err)
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}

		logger.Debug("Query received", "query", req.Query, "parameters", len(req.Parameters))

		// Parse the raw query string into an executable query, binding any
		// vector placeholders (vec:$v) from the request parameters.
		q, err := query.Parse[K, P](req.Query, req.Parameters, s.Config)

		if err != nil {
			logger.Warn("Rejecting unparsable query", "query", req.Query, "error", err)
			status, msg := errorToResponse(err)
			c.JSON(status, ErrorResponse{Error: msg})
			return
		}

		// Reject a selector outside the allocated range up front: it is a
		// client error, and letting it reach the scheduler would otherwise
		// fail deep in Select.
		if int(q.GetGraphID()) >= s.DB.NumGraphs() {
			logger.Warn("Rejecting out-of-range graph selector",
				"graph", q.GetGraphID(), "max", s.DB.NumGraphs()-1)
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: fmt.Sprintf("graph %d does not exist (valid range 0-%d)",
					q.GetGraphID(), s.DB.NumGraphs()-1),
			})
			return
		}

		// Build an execution plan (stream) for the parsed query.
		stream, err := s.Engine.Plan(q)

		if err != nil {
			logger.Error("Failed to plan query", "query", req.Query, "error", err)
			status, msg := errorToResponse(err)
			c.JSON(status, ErrorResponse{Error: msg})
			return
		}

		logger.Debug("Query planned, dispatching to engine",
			"graph", q.GetGraphID(), "write", q.IsWrite())

		// Execute the plan asynchronously against the engine.
		s.Engine.Apply(stream)

		// Wait for the stream to finish, then return results or the error.
		// If the client goes away first, stop waiting.
		select {
		case <-stream.Done():
			if stream.Err != nil {
				logger.Error("Query execution failed", "query", req.Query, "error", stream.Err)
				status, msg := errorToResponse(stream.Err)
				c.JSON(status, ErrorResponse{Error: msg})
				return
			}
			logger.Info("Query executed", "query", req.Query, "graph", q.GetGraphID())
			c.JSON(http.StatusOK, gin.H{
				"results": stream.Result,
			})
		case <-c.Request.Context().Done():
			logger.Warn("Client disconnected before query completed", "query", req.Query)
			return
		}
	}
}
