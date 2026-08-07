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
	"net/http"
	"strconv"

	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/internal/hash"

	"github.com/RonsenbergVI/fraise/pkg/db"
	"github.com/RonsenbergVI/fraise/pkg/engine"
	"github.com/RonsenbergVI/fraise/pkg/logger"

	"github.com/gin-gonic/gin"
)

// Server ties together the HTTP layer, the database, and the query engine.
// K is the key type used to identify records, and P is the floating-point
// precision (float32 or float64) used for the values they hold.
type Server[K ~uint64, P float32 | float64] struct {
	// Config holds the full application configuration.
	Config *config.ConfigSet

	// DB is the underlying data store.
	DB *db.DB[K, P]
	// Engine executes queries against the data store.
	Engine *engine.Engine[K, P]

	// router dispatches incoming HTTP requests to their handlers.
	router *gin.Engine

	// httpServer is the explicit HTTP server carrying the request timeouts and
	// enabling a graceful shutdown that router.Run cannot provide.
	httpServer *http.Server
}

// New constructs a Server wired up with a database, an engine, and the HTTP
// routes. The hasher determines how keys are mapped to their string
// representation for storage and lookup.
func New[K ~uint64, P float32 | float64](config *config.ConfigSet, hasher hash.Hasher[K, string]) (*Server[K, P], error) {

	// Initialise the data store from the configuration.
	db, err := db.NewDB[K, P](config)
	if err != nil {
		return nil, err
	}
	// Initialise the query engine with the same configuration and hasher.
	engine := engine.NewEngine[K, P](config, hasher)

	// The scheduler executes streams against the data store.
	engine.Scheduler.DB = db

	if gin.Mode() == gin.DebugMode {
		gin.SetMode(gin.ReleaseMode)
	}

	s := &Server[K, P]{
		Config: config,
		DB:     db,
		Engine: engine,
		router: gin.Default(),
	}

	// The service terminates TLS/proxying elsewhere; it never trusts client
	// address headers, so no proxies are trusted (this also silences gin's
	// default-trust-all startup warning).
	if err := s.router.SetTrustedProxies(nil); err != nil {
		return nil, err
	}

	// Register the HTTP routes before the server is returned.
	s.setupRoutes()

	// Build an explicit HTTP server so the connection has read/write/idle
	// timeouts and can be shut down gracefully (router.Run offers neither).
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
// server stops on its own, then tears the stack down gracefully: in-flight
// requests are given ShutdownGrace to finish before the engine and database are
// stopped. It returns an error if any component fails to start or the HTTP
// server fails for a reason other than a clean shutdown.
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

	// Serve in the background so we can react to ctx cancellation. A clean
	// shutdown surfaces as http.ErrServerClosed, which is not a failure.
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

// Stop gracefully shuts down the engine and then the database. The engine is
// stopped first so its workers have drained before DB.Stop reallocates the
// graph slice; the reverse order would let a running worker dereference a nil
// graph. It returns an error if the database fails to stop.
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
// versioned query endpoints under /api/v1.
func (s *Server[K, P]) setupRoutes() {

	// Group versioned API endpoints under /api/v1.
	v1 := s.router.Group("/api/v1")

	// Root health check endpoint.
	s.router.GET("/", s.handleHealthCheck())

	// Query endpoint: /q for queries. The body-size cap runs before the handler
	// so an over-large body is rejected before it is buffered.
	v1.POST("/q", limitBody(s.Config.Server.MaxBodyBytes), s.handleQuery())

	// Stats endpoint: per-graph snapshots (nodes, edges, vectors, forest).
	v1.GET("/stats", s.handleStats())
}

// limitBody caps how much of a request body a handler will read: it wraps the
// body in an http.MaxBytesReader, so reading past the limit fails and the
// binding handler returns 400 rather than buffering an unbounded body.
func limitBody(max int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max)
		c.Next()
	}
}
