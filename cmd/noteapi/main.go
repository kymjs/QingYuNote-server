package main

import (
	"log"
	"net/http"

	"github.com/kymjs/noteapi/internal/api"
	"github.com/kymjs/noteapi/internal/config"
	"github.com/kymjs/noteapi/internal/store"
)

func main() {
	cfg := config.Load()
	st, err := store.OpenMySQL(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("mysql: %v", err)
	}
	defer st.DB.Close()

	srv, err := api.NewServer(cfg, st)
	if err != nil {
		log.Fatalf("server: %v", err)
	}

	log.Printf("listening %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, srv.Routes()); err != nil {
		log.Fatal(err)
	}
}
