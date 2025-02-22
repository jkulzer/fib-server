package db

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/rs/zerolog/log"

	"github.com/jkulzer/fib-server/models"
)

func InitDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open("sqlite.db"), &gorm.Config{})
	if err != nil {
		log.Err(err).Msg("failed to create/open db")
	}

	db.AutoMigrate(&models.UserAccount{})
	db.AutoMigrate(&models.Session{})
	db.AutoMigrate(&models.Lobby{})
	db.AutoMigrate(&models.HistoryInDB{})
	db.AutoMigrate(&models.CardDraw{})
	db.AutoMigrate(&models.CurrentDraw{})
	db.AutoMigrate(&models.Card{})

	db.Session(&gorm.Session{FullSaveAssociations: true})

	return db
}
