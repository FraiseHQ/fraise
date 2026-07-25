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
	"net/http"

	"github.com/RonsenbergVI/fraise/internal/query"
	"github.com/gin-gonic/gin"
)

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
		var req HandleQueryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Parse the raw query string into an executable query.
		q, err := query.Parse[K, P](req.Query, s.Config)

		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Build an execution plan (stream) for the parsed query.
		stream, err := s.Engine.Plan(q)

		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Execute the plan asynchronously against the engine.
		s.Engine.Apply(stream)

		// Wait for the stream to finish, then return results or the error.
		// If the client goes away first, stop waiting.
		select {
		case <-stream.Done():
			if stream.Err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": stream.Err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"results": stream.Result,
			})
		case <-c.Request.Context().Done():
			return
		}
	}
}

// handleQueryWithParameters returns the handler for parameterised queries.
// It is not yet implemented.
func (s *Server[K, P]) handleQueryWithParameters() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "parameterised queries are not implemented yet"})
	}
}
