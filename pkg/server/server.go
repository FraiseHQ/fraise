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
}

// New constructs a Server wired up with a database, an engine, and the HTTP
// routes. The hasher determines how keys are mapped to their string
// representation for storage and lookup.
func New[K ~uint64, P float32 | float64](config *config.ConfigSet, hasher hash.Hasher[K, string]) *Server[K, P] {

	// Initialise the data store from the configuration.
	db, err := db.NewDB[K, P](config)
	if err != nil {

	}
	// Initialise the query engine with the same configuration and hasher.
	engine := engine.NewEngine[K, P](config, hasher)

	// The scheduler executes streams against the data store.
	engine.Scheduler.DB = db

	s := &Server[K, P]{
		Config: config,
		DB:     db,
		Engine: engine,
		router: gin.Default(),
	}
	// Register the HTTP routes before the server is returned.
	s.setupRoutes()
	return s
}

// Start brings up the database and engine, then begins serving HTTP requests
// on the configured port. It blocks until the HTTP server stops, and returns
// an error if any component fails to start.
func (s *Server[K, P]) Start() error {
	logger.Info("Starting database")
	err := s.DB.Start()
	if err != nil {
		logger.Info("Failed to start database!")
		return ErrUnableToStartDatabase
	}
	logger.Info("Starting Engine")
	s.Engine.Start()
	if err != nil {
		logger.Info("Failed to start engine!")
		return ErrUnableToStartEngine
	}
	// Block serving HTTP requests on the configured port.
	err = s.router.Run(":" + strconv.Itoa(s.Config.Server.Port))
	if err != nil {
		logger.Info("Failed to start server!")
		return ErrUnableToStartEngine
	}
	return nil
}

// Stop gracefully shuts down the database and engine. It returns an error if
// the database fails to stop.
func (s *Server[K, P]) Stop() error {
	logger.Info("Stopping database")
	err := s.DB.Stop()
	if err != nil {
		logger.Info("Failed to stop database!")
		return ErrUnableToStopDatabase
	}

	logger.Info("Stopping engine")
	s.Engine.Stop()
	return nil
}

// setupRoutes registers the HTTP endpoints: a root health check plus the
// versioned query endpoints under /api/v1.
func (s *Server[K, P]) setupRoutes() {

	// Group versioned API endpoints under /api/v1.
	v1 := s.router.Group("/api/v1")

	// Root health check endpoint.
	s.router.GET("/", s.handleHealthCheck())

	// Query endpoints: /q for plain queries, /qp for parameterised queries.
	v1.POST("/q", s.handleQuery())
	v1.POST("/qp", s.handleQueryWithParameters())
}
