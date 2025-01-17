package sharedModels

import (
	"github.com/google/uuid"
	"time"
)

type LoginInfo struct {
	Username string
	Password string
}

type SessionToken struct {
	Token  uuid.UUID
	Expiry time.Time
}

type LobbyCreationResponse struct {
	LobbyToken string
}

type LobbyJoinRequest struct {
	LobbyToken string
}

type UserRole int

const (
	Hider UserRole = iota
	Seeker
)

type RoleAvailability []UserRole

var LobbyCodeRegex = "^[A-Z0-9]{6}$"
