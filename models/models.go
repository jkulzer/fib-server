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
	Token         uuid.UUID
	UserAccountID uint
	UserAccount   UserAccount
	Expiry        time.Time
}
