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

// Package middleware defines shared middleware for handlers.
package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/google/exposure-notifications-verification-server/internal/auth"
	"github.com/google/exposure-notifications-verification-server/pkg/cache"
	"github.com/google/exposure-notifications-verification-server/pkg/controller"
	"github.com/google/exposure-notifications-verification-server/pkg/database"
	"github.com/google/exposure-notifications-verification-server/pkg/render"

	"github.com/google/exposure-notifications-server/pkg/logging"

	"github.com/gorilla/mux"
)

// RequireAuth requires a user to be logged in. It also ensures that currentUser
// is set in the template map. It fetches a user from the session and stores the
// full record in the request context.
func RequireAuth(ctx context.Context, cacher cache.Cacher, authProvider auth.Provider, db *database.Database, h *render.Renderer, sessionIdleTTL, expiryCheckTTL time.Duration) mux.MiddlewareFunc {
	logger := logging.FromContext(ctx).Named("middleware.RequireAuth")

	cacheTTL := 15 * time.Minute

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			session := controller.SessionFromContext(ctx)
			if session == nil {
				logger.Errorw("session does not exist")
				controller.MissingSession(w, r, h)
				return
			}

			flash := controller.Flash(session)

			// Check session idle timeout.
			if t := controller.LastActivityFromSession(session); !t.IsZero() {
				// If it's been more than the TTL since we've seen this session,
				// "expire" it by creating a new empty session. Note that we don't force
				// the user back to a login page or anything - other middlewares will
				// handle that if needed.
				if time.Since(t) > sessionIdleTTL {
					logger.Debug("session is expired")
					controller.Unauthorized(w, r, h)
					return
				}
			}

			// Get the email from the auth provider.
			email, err := authProvider.EmailAddress(ctx, session)
			if err != nil {
				authProvider.ClearSession(ctx, session)

				logger.Debugw("failed to get email from session", "error", err)
				flash.Error("An error occurred trying to verify your credentials.")
				controller.Unauthorized(w, r, h)
				return
			}

			// Load the user by using the cache to alleviate pressure on the database
			// layer.
			var user database.User
			cacheKey := &cache.Key{
				Namespace: "users:by_email",
				Key:       email,
			}
			if err := cacher.Fetch(ctx, cacheKey, &user, cacheTTL, func() (interface{}, error) {
				return db.FindUserByEmail(email)
			}); err != nil {
				authProvider.ClearSession(ctx, session)

				if database.IsNotFound(err) {
					logger.Debugw("user does not exist")
					controller.Unauthorized(w, r, h)
					return
				}

				logger.Errorw("failed to lookup user", "error", err)
				controller.InternalError(w, r, h, err)
				return
			}

			// Check if the session is still valid.
			if time.Now().After(user.LastRevokeCheck.Add(expiryCheckTTL)) {
				// Check if the session has been revoked.
				if err := authProvider.CheckRevoked(ctx, session); err != nil {
					authProvider.ClearSession(ctx, session)

					logger.Debugw("session revoked", "error", err)
					controller.Unauthorized(w, r, h)
					return
				}

				// Update the revoke check time.
				if err := db.TouchUserRevokeCheck(&user); err != nil {
					logger.Errorw("failed to update revocation check time", "error", err)
					controller.InternalError(w, r, h, err)
					return
				}

				// Update the user in the cache so it has the new revoke check time.
				if err := cacher.Write(ctx, cacheKey, &user, cacheTTL); err != nil {
					logger.Errorw("failed to cached user revocation check time", "error", err)
					controller.InternalError(w, r, h, err)
					return
				}
			}

			// Save the user on the context.
			ctx = controller.WithUser(ctx, &user)
			*r = *r.WithContext(ctx)

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAdmin requires the current user is a global administrator. It must
// come after RequireAuth so that a user is set on the context.
func RequireAdmin(ctx context.Context, h *render.Renderer) mux.MiddlewareFunc {
	logger := logging.FromContext(ctx).Named("middleware.RequireAdminHandler")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			currentUser := controller.UserFromContext(ctx)
			if currentUser == nil {
				controller.MissingUser(w, r, h)
				return
			}

			if !currentUser.Admin {
				logger.Debugw("user is not an admin")
				controller.Unauthorized(w, r, h)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
