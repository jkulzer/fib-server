package main

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/rs/zerolog/log"

	chi "github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/jkulzer/fib-server/db"
	"github.com/jkulzer/fib-server/geo"
	"github.com/jkulzer/fib-server/routes"
)

func main() {
	port := 3001

	db := db.InitDB()

	r := chi.NewRouter()

	r.Use(middleware.Logger)

	routes.Router(r, db)

	geo.ProcessData()

	fmt.Println("Listening on :" + strconv.Itoa(port))
	err := http.ListenAndServe("0.0.0.0:"+strconv.Itoa(port), r)
	if err != nil {
		log.Err(err)
	}
}
