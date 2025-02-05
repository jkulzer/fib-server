package routes

import (
	"context"
	// "fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/jkulzer/fib-server/models"

	"gorm.io/gorm"
)

var nullUuidString = "00000000-0000-0000-0000-000000000000"

func AuthMiddleware(db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Missing Authorization header", http.StatusUnauthorized)
				return
			}

			// Check for Bearer token format
			if !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, "Invalid Authorization header format", http.StatusUnauthorized)
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == nullUuidString {
				http.Error(w, "User token is null", http.StatusBadRequest)
			}

			parsedToken, err := uuid.Parse(token)
			if err != nil {
				http.Error(w, "Failed to parse token", http.StatusUnauthorized)
			}

			// log.Debug().Msg("user token: " + token)

			var session models.Session
			// result := db.Where("token = ?", parsedToken.String()).First(&models.Session{})
			result := db.Where(&models.Session{Token: parsedToken}).First(&session)
			// log.Debug().Msg("found session has token " + fmt.Sprint(session.Token))
			// log.Debug().Msg("found session has user id " + fmt.Sprint(session.UserAccountID))
			// if the user creation fails,
			if result.Error != nil || session.Token.String() == nullUuidString || token == nullUuidString {
				log.Info().Msg("failed to find token, unauthenticated")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write(nil)
			} else {
				// log.Info().Msg("authenticated user with id " + fmt.Sprint(session.UserAccountID))
				ctx := context.WithValue(r.Context(), models.UserIDKey, session.UserAccountID)
				// Token is valid; proceed to the next handler
				next.ServeHTTP(w, r.WithContext(ctx))
			}
		})
	}
}
