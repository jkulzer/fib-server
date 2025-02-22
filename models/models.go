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
	History             []HistoryInDB `gorm:"foreignKey:LobbyID"`
	HiderDeck           []Card        `gorm:"foreignKey:HiderDeckLobbyID"`
	// opportunities to draw cards
	CardDraws []CardDraw `gorm:"foreignKey:LobbyID"`
	// drawn cards from which the selection hasn't been made
	CurrentDraw CurrentDraw `gorm:"foreignKey:LobbyID"`
	// the card list from which cards get drawn
	RemainingCards []Card `gorm:"foreignKey:RemainingCardsLobbyID"`
}

type HistoryInDB struct {
	gorm.Model
	LobbyID     uint
	LobbyType   string
	Title       string
	Description string
}

type CurrentDraw struct {
	gorm.Model
	LobbyID uint
	Cards   []Card `gorm:"foreignKey:CurrentDrawID"`
	ToPick  uint
}

type Card struct {
	gorm.Model
	HiderDeckLobbyID      uint
	RemainingCardsLobbyID uint
	CurrentDrawID         uint
	Title                 string
	Description           string
	Type                  sharedModels.CardType
	ExpirationDuration    time.Duration
	ActivationTime        time.Time
	BonusTime             time.Duration
}

func (c *Card) DTO() sharedModels.Card {
	return sharedModels.Card{
		IDInDB:             c.ID,
		Title:              c.Title,
		Description:        c.Description,
		Type:               c.Type,
		ExpirationDuration: c.ExpirationDuration,
		ActivationTime:     c.ActivationTime,
		BonusTime:          c.BonusTime,
	}

}

type CardDraw struct {
	gorm.Model
	LobbyID     uint
	CardsToDraw uint
	CardsToPick uint
}

type ContextKey uint

const (
	UserIDKey ContextKey = iota
	LobbyKey
)
