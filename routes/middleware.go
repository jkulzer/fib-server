package routes

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	chi "github.com/go-chi/chi/v5"

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
			result := db.Where(&models.Session{Token: parsedToken}).First(&session)
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

func LobbyMiddleware(db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			lobbyToken := chi.URLParam(r, "index")
			// regex for verifying the lobby token
			lobbyTokenRegex := regexp.MustCompile("^[A-Z0-9]{6}$")
			// if the input is valid
			if !lobbyTokenRegex.MatchString(lobbyToken) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write(nil)
				return
			}
			// finds lobby in DB
			var lobby models.Lobby
			result := db.Where("token = ?", lobbyToken).First(&lobby).Preload("history_in_dbs")
			// if lobby can't be found
			if result.Error != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write(nil)
				return
			}
			ctx := context.WithValue(r.Context(), models.LobbyKey, lobby)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
