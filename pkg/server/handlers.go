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

func (s *Server[K, P]) handleHealthCheck() gin.HandlerFunc {
	return nil
}

func (s *Server[K, P]) handleQuery() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req HandleQueryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error", err.Error()})
			return
		}

		q := query.Parse[K, P](req.Query)

		stream, err := s.Engine.Plan(q)

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error", err.Error()})
			return
		}

		s.Engine.Apply(stream)

		select {
		case <-stream.Done():
			if stream.Err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error", stream.Err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"results": stream.Result,
			})
		case <-stream.Done():
			return
		}

	}
}

func (s *Server[K, P]) handleQueryWithParameters() gin.HandlerFunc {
	return nil
}
