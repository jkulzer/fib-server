package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

type UserAccount struct {
	gorm.Model
	Name     string `gorm:"unique"`
	Password string
	Sessions []Session
}

type Session struct {
	gorm.Model
	Token         uuid.UUID `gorm:"unique"`
	UserAccountID uint
	UserAccount   UserAccount
	Expiry        time.Time
}

type Lobby struct {
	gorm.Model
	Token     string `gorm:"unique"`
	CreatorID uint
	Creator   UserAccount `gorm:"foreignKey:CreatorID"`
	HiderID   uint
	Hider     UserAccount `gorm:"foreignKey:CreatorID"`
	SeekerID  uint
	Seeker    UserAccount `gorm:"foreignKey:SeekerID"`
}

type ContextKey uint

const UserIDKey ContextKey = iota
