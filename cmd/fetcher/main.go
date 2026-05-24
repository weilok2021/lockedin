package main

import (
	"log"

	"github.com/weilok2021/lockedin/internal/config"
)

func main() {
	// cmd/fetcher/main.go
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("starting server on %s\n", cfg.Port)
}
