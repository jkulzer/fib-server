package routes

import (
	"gorm.io/gorm"

	"errors"
	"fmt"

	"net/http"
	"net/url"

	"github.com/jkulzer/fib-server/controllers"
	"github.com/jkulzer/fib-server/helpers"
	"github.com/jkulzer/fib-server/models"

	chi "github.com/go-chi/chi/v5"
)

func Router(r chi.Router, db *gorm.DB) {
	r.Route("/register", func(r chi.Router) {
		r.Get("/",
			func(w http.ResponseWriter, r *http.Request) {
				isLoggedIn, _ := controllers.GetLoginFromSession(db, r)
			},
		)

		r.Post("/",
			func(w http.ResponseWriter, r *http.Request) {
				response, err := helpers.ReadHttpResponse(r.Body)
				data, err := url.ParseQuery(response)
				if err != nil {
					fmt.Println("Failed to parse query")
				}

				password := data["password"][0]

				hashedPassword, err := controllers.HashPassword(password)
				if err != nil {
					fmt.Println("Failed to hash password")
				}

				userName := models.UserAccount{
					Name:     data["username"][0],
					Password: hashedPassword,
				}
				// tries to create the user in the db
				result := db.Create(&userName)

				// if the user creation fails,
				if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
					fmt.Println("Duplicate Username")
					isLoggedIn, _ := controllers.GetLoginFromSession(db, r)
				} else {
					controllers.CreateSession(db, userName, w)
				}
			},
		)
	})
}
