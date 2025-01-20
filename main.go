package main

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/rs/zerolog/log"

	chi "github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/swaggo/http-swagger/v2"

	"github.com/jkulzer/fib-server/db"
	_ "github.com/jkulzer/fib-server/docs"
	"github.com/jkulzer/fib-server/routes"
	// "github.com/jkulzer/fib-server/geo"
)

//	@title		FiB-Server API
//	@version	1.0

//	@license.name	AGPL

// @BasePath	/
func main() {
	port := 3001

	db := db.InitDB()

	r := chi.NewRouter()

	r.Use(middleware.Logger)

	routes.Router(r, db)

	// geo.ProcessData()

	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("http://localhost:3001/swagger/doc.json"), //The url pointing to API definition
	))

	fmt.Println("Listening on :" + strconv.Itoa(port))
	err := http.ListenAndServe("0.0.0.0:"+strconv.Itoa(port), r)
	if err != nil {
		log.Err(err)
	}
}
