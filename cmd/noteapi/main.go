package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/kymjs/noteapi/internal/api"
	"github.com/kymjs/noteapi/internal/config"
	"github.com/kymjs/noteapi/internal/store"
)

func main() {
	cfg := config.Load()

	// 默认日志到 stderr（systemd / Docker / act_runner 可采集）；可选同时追加到文件便于配合 logrotate。
	if path := strings.TrimSpace(os.Getenv("LOG_FILE")); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatalf("open LOG_FILE %s: %v", path, err)
		}
		defer f.Close()
		log.SetOutput(io.MultiWriter(os.Stderr, f))
		log.Printf("noteapi: logging to stderr and %s", path)
	}

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
