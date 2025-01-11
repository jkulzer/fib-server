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
