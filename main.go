package main

import (
	"fmt"
	"strconv"

	chi "github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/jkulzer/fib-server/db"
	"github.com/jkulzer/fib-server/routes"
)

func main() {
	port := 3000

	db := db.InitDB()

	fmt.Println("Listening on :" + strconv.Itoa(port))
	r := chi.NewRouter()

	r.Use(middleware.Logger)

	routes.Router(r, db)
}
