// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package login defines the controller for the login page.
package login

import (
	"context"

	"github.com/google/exposure-notifications-verification-server/internal/auth"
	"github.com/google/exposure-notifications-verification-server/pkg/config"
	"github.com/google/exposure-notifications-verification-server/pkg/database"
	"github.com/google/exposure-notifications-verification-server/pkg/render"

	"github.com/google/exposure-notifications-server/pkg/logging"

	"go.uber.org/zap"
)

type Controller struct {
	authProvider auth.Provider
	config       *config.ServerConfig
	db           *database.Database
	h            *render.Renderer
	logger       *zap.SugaredLogger
}

// New creates a new login controller.
func New(
	ctx context.Context,
	authProvider auth.Provider,
	config *config.ServerConfig,
	db *database.Database,
	h *render.Renderer) *Controller {
	logger := logging.FromContext(ctx).Named("login")

	return &Controller{
		authProvider: authProvider,
		config:       config,
		db:           db,
		h:            h,
		logger:       logger,
	}
}
