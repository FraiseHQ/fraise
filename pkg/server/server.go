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
	"github.com/RonsenbergVI/fraise/internal/config"
	"github.com/RonsenbergVI/fraise/internal/db"

	"github.com/RonsenbergVI/fraise/pkg/engine"
	"github.com/RonsenbergVI/fraise/pkg/scheduler"

	"github.com/gin-gonic/gin"
)

type Server[K comparable, P float32 | float64] struct {
	Config *config.ConfigSet

	DB        *db.DB[K, P]
	Engine    *engine.Engine[K, P]
	Scheduler *scheduler.Scheduler[K, P]

	router *gin.Engine
}

func New[K comparable, P float32 | float64](config *config.ConfigSet) *Server[K, P] {

	db, err := db.NewDB[K, P](config)
	if err != nil {

	}
	scheduler := scheduler.NewScheduler[K, P](config)

	engine, err := engine.NewEngine[K, P](config)
	if err != nil {

	}

	s := &Server[K, P]{
		DB:        db,
		Engine:    engine,
		Scheduler: scheduler,
		router:    gin.Default(),
	}
	s.setupRoutes()
	return s
}

func (s *Server[K, P]) setupRoutes() {
	s.router.GET("/", s.handleHealthCheck())
	s.router.GET("/q", s.handleQuery())
	s.router.GET("/qp", s.handleQueryWithParameters())
}

func (s *Server[K, P]) Start() error {
	err := s.DB.Start()
	if err != nil {
		return ErrUnableToStartDatabase
	}
	s.Scheduler.Start()
	if err != nil {
		return ErrUnableToStartScheduler
	}
	s.Engine.Start()
	if err != nil {
		return ErrUnableToStartEngine
	}
	return nil
}

func (s *Server[K, P]) Stop() error {
	err := s.DB.Stop()
	if err != nil {
		return ErrUnableToStopDatabase
	}
	s.Scheduler.Stop()
	err = s.Engine.Stop()
	if err != nil {
		return ErrUnableToStopEngine
	}
	return nil
}
