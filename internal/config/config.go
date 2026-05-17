package config

import (
	"errors"
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
}

type ServerConfig struct {
	Port        string
	Environment string
}

type DatabaseConfig struct {
	URL string
}

func Load() (Config, error) {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file, using process env")
	}
	cfg := Config{
		Server:   ServerConfig{Port: os.Getenv("PORT"), Environment: os.Getenv("ENV")},
		Database: DatabaseConfig{URL: os.Getenv("DB_URL")},
	}
	if cfg.Database.URL == "" {
		return cfg, errors.New("DB_URL required")
	}
	return cfg, nil
}
