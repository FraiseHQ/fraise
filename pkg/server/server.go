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
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/FraiseHQ/fraise/internal/config"
	"github.com/FraiseHQ/fraise/internal/hash"

	"github.com/FraiseHQ/fraise/pkg/db"
	"github.com/FraiseHQ/fraise/pkg/engine"
	"github.com/FraiseHQ/fraise/pkg/logger"

	"github.com/gin-gonic/gin"
)

// Server ties together the HTTP layer, the database, and the query engine.
// K is the node key type, and P the floating-point precision (float32 or
// float64) of embeddings and scores.
type Server[K ~uint64, P float32 | float64] struct {
	// Config holds the full application configuration.
	Config *config.ConfigSet

	// DB is the underlying data store.
	DB *db.DB[K, P]
	// Engine executes queries against the data store.
	Engine *engine.Engine[K, P]

	router *gin.Engine

	// httpServer carries the request timeouts and allows the graceful shutdown
	// that router.Run cannot provide.
	httpServer *http.Server
}

// New constructs a Server wired up with a database, an engine, and the HTTP
// routes. The hasher keys the engine's plan cache. Each graph derives its node
// keys with its own hasher.
func New[K ~uint64, P float32 | float64](config *config.ConfigSet, hasher hash.Hasher[K, string]) (*Server[K, P], error) {

	db, err := db.NewDB[K, P](config)
	if err != nil {
		return nil, err
	}
	engine := engine.NewEngine[K, P](config, hasher)

	engine.Scheduler.DB = db

	if gin.Mode() == gin.DebugMode {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	// Every failure answers in the documented {"error": ...} shape. A recovered
	// panic is an internal failure like any other. It gets a generic body, with
	// the detail in the log. gin's own recovery answers 500 with no body.
	r.Use(gin.CustomRecovery(func(c *gin.Context, recovered any) {
		logger.Error("Recovered from panic in handler", "path", c.Request.URL.Path, "panic", recovered)
		c.AbortWithStatusJSON(http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
	}))

	s := &Server[K, P]{
		Config: config,
		DB:     db,
		Engine: engine,
		router: r,
	}

	// The service never trusts client address headers, so no proxy is trusted.
	// TLS and proxying are handled elsewhere. This also silences gin's startup
	// warning about trusting every proxy by default.
	if err := s.router.SetTrustedProxies(nil); err != nil {
		return nil, err
	}

	s.setupRoutes()

	s.httpServer = &http.Server{
		Addr:              ":" + strconv.Itoa(config.Server.Port),
		Handler:           s.router,
		ReadTimeout:       config.Server.ReadTimeout,
		ReadHeaderTimeout: config.Server.ReadHeaderTimeout,
		WriteTimeout:      config.Server.WriteTimeout,
		IdleTimeout:       config.Server.IdleTimeout,
	}
	return s, nil
}

// Start brings up the database and engine, then serves HTTP requests on the
// configured port. It blocks until ctx is cancelled (e.g. SIGTERM) or the HTTP
// server stops on its own. It then tears the stack down gracefully. In-flight
// requests get ShutdownGrace to finish before the engine and database stop. It
// returns an error if any component fails to start, or if the HTTP server fails
// for a reason other than a clean shutdown.
func (s *Server[K, P]) Start(ctx context.Context) error {
	logger.Info("Starting database")
	if err := s.DB.Start(); err != nil {
		logger.Error("Failed to start database", "error", err)
		return ErrUnableToStartDatabase
	}
	logger.Info("Starting engine")
	if err := s.Engine.Start(); err != nil {
		logger.Error("Failed to start engine", "error", err)
		return ErrUnableToStartEngine
	}

	// Serve in the background so Start can react to ctx cancellation. A clean
	// shutdown returns http.ErrServerClosed, which is not a failure.
	serveErr := make(chan error, 1)
	go func() {
		logger.Info("Serving HTTP", "port", s.Config.Server.Port)
		err := s.httpServer.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serveErr <- err
	}()

	select {
	case err := <-serveErr:
		// The server stopped on its own (e.g. the port was already bound). Tear
		// down what we started before returning.
		if err != nil {
			logger.Error("HTTP server stopped", "error", err)
		}
		if stopErr := s.Stop(); stopErr != nil {
			logger.Error("Failed to stop cleanly", "error", stopErr)
		}
		if err != nil {
			return ErrUnableToServe
		}
		return nil
	case <-ctx.Done():
		logger.Info("Shutdown signal received, draining in-flight requests",
			"grace", s.Config.Server.ShutdownGrace)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.Config.Server.ShutdownGrace)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("HTTP graceful shutdown timed out", "error", err)
		}
		return s.Stop()
	}
}

// Stop gracefully shuts down the engine and then the database. The engine stops
// first so its workers have drained before DB.Stop reallocates the graph slice.
// The reverse order would let a running worker dereference a nil graph. It
// returns an error if the database fails to stop.
func (s *Server[K, P]) Stop() error {
	logger.Info("Stopping engine")
	s.Engine.Stop()

	logger.Info("Stopping database")
	if err := s.DB.Stop(); err != nil {
		logger.Error("Failed to stop database", "error", err)
		return ErrUnableToStopDatabase
	}
	return nil
}

// setupRoutes registers the HTTP endpoints: a root health check plus the
// versioned API under /api/v1.
func (s *Server[K, P]) setupRoutes() {

	v1 := s.router.Group("/api/v1")

	s.router.GET("/", s.handleHealthCheck())

	// The body-size cap runs before the handler, so an over-large body is
	// rejected before it is buffered.
	v1.POST("/q", limitBody(s.Config.Server.MaxBodyBytes), s.handleQuery(false))

	v1.POST("/explain", limitBody(s.Config.Server.MaxBodyBytes), s.handleQuery(true))

	v1.GET("/stats", s.handleStats())

	// An unknown path, or a known one with the wrong method, is a 404. It
	// answers in the same {"error": ...} shape as every other failure, naming
	// the method and path. gin's default answers in plain text.
	s.router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: fmt.Sprintf("no endpoint %s %s", c.Request.Method, c.Request.URL.Path)})
	})
}

// limitBody caps how much of a request body a handler reads. Reading past max
// fails, so the binding handler answers 400 instead of buffering an unbounded
// body.
func limitBody(max int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max)
		c.Next()
	}
}
