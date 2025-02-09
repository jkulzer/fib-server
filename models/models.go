package models

import (
	"github.com/google/uuid"

	"gorm.io/gorm"

	"time"

	"github.com/jkulzer/fib-server/sharedModels"
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
	Token               string `gorm:"unique"`
	CreatorID           uint
	Creator             UserAccount `gorm:"foreignKey:CreatorID"`
	HiderID             uint
	Hider               UserAccount `gorm:"foreignKey:CreatorID"`
	SeekerID            uint
	Seeker              UserAccount `gorm:"foreignKey:SeekerID"`
	Phase               sharedModels.GamePhase
	HiderReady          bool
	SeekerReady         bool
	RunStartTime        time.Time
	ZoneCenterLat       float64
	ZoneCenterLon       float64
	HiderLat            float64
	HiderLon            float64
	SeekerLat           float64
	SeekerLon           float64
	ExcludedArea        string
	ThermometerDistance float64
	ThermometerStartLat float64
	ThermometerStartLon float64
}

type ContextKey uint

const (
	UserIDKey ContextKey = iota
	LobbyKey
)
