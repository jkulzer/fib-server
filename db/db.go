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

	return db
}
