package config

import (
	"errors"
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port        string
	Environment string
	DbURL       string
}

func Load() (Config, error) {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file, using process env")
	}
	cfg := Config{
		Port:        os.Getenv("PORT"),
		Environment: os.Getenv("ENV"),
		DbURL:       os.Getenv("DB_URL"),
	}
	if cfg.DbURL == "" {
		return cfg, errors.New("DB_URL required")
	}
	return cfg, nil
}
