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

type CreateLobby struct {
	Token uuid.UUID
}

type LobbyCreationResponse struct {
	LobbyToken string
}

type LobbyJoinRequest struct {
	LobbyToken string
}
