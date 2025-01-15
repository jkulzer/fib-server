package controllers

import (
	"gorm.io/gorm"

	"github.com/google/uuid"

	"github.com/jkulzer/fib-server/models"

	"errors"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"time"
)

func IsExpired(s models.Session) bool {
	return s.Expiry.Before(time.Now())
}

func NewSession(db *gorm.DB, userAccount models.UserAccount) (uuid.UUID, time.Duration) {
	sessionToken := uuid.New()
	// 5 min expiry time
	expiryDuration := 12 * time.Hour
	expiresAt := time.Now().Add(expiryDuration)

	db.Create(&models.Session{
		Token:         sessionToken,
		UserAccountID: userAccount.ID,
		Expiry:        expiresAt,
	})
	return sessionToken, expiryDuration
}

func GetLoginFromSession(db *gorm.DB, r *http.Request) (bool, models.Session) {
	cookie, err := r.Cookie("Session")
	if err != nil {
		return false, models.Session{} // returns empty UserAccount struct
	}

	var session models.Session

	sessionToken, err := uuid.Parse(cookie.Value)
	if err != nil {
		fmt.Println("Failed to parse UUID")
	}

	db.Preload("UserAccount").Where(&models.Session{Token: sessionToken}).First(&session)
	// checks if the token in the cookie is in any active session
	result := db.Where(&models.Session{Token: sessionToken}).First(&session)
	if result.Error != nil {
		return false, models.Session{} // returns empty UserAccount struct
	} else {
		if session.Expiry.After(time.Now()) {
			return true, session
		} else {
			fmt.Println("Session expired")
			return false, models.Session{}
		}
	}

}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func RemoveSession(db *gorm.DB, w http.ResponseWriter, r *http.Request) models.UserAccount {
	_, session := GetLoginFromSession(db, r)

	user := session.UserAccount

	db.Delete(&session)

	// deletes the cookie
	cookie := http.Cookie{
		Name:     "Session",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
	}
	http.SetCookie(w, &cookie)

	return user
}

func GetSessionsForUser(db *gorm.DB, r *http.Request, session models.Session) []models.Session {

	var sessionList []models.Session
	result := db.Find(&sessionList).Where(models.Session{UserAccountID: session.UserAccountID})
	if result.Error != nil {
		fmt.Println("Failed to get all user sessions for user " + session.UserAccount.Name)
	}

	return sessionList
}

func ClearOutExpiredSessions(db *gorm.DB) {
	fmt.Println("Clearing out old sessions")
	var sessionList []models.Session
	result := db.Find(&sessionList)
	currentTime := time.Now()
	if result.Error != nil {
		fmt.Println("Can't get list of sessions")
	} else {
		for _, session := range sessionList {
			if session.Expiry.Before(currentTime) {
				db.Delete(&session)
			}
		}
	}
}

func DeleteSessionByUuid(uuid uuid.UUID, db *gorm.DB, r *http.Request) error {
	isLoggedIn, session := GetLoginFromSession(db, r)
	if isLoggedIn {
		var toBeDeletedSession models.Session
		db.Model(models.Session{Token: uuid, UserAccountID: session.UserAccount.ID}).First(&toBeDeletedSession)
		fmt.Println("Will delete session " + toBeDeletedSession.Token.String())
		db.Delete(&toBeDeletedSession)
		return nil
	} else {
		return errors.New("Not logged in")
	}
}
