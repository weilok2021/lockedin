package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/weilok2021/lockedin/internal/config"
	"github.com/weilok2021/lockedin/internal/db"
)

type App struct {
	queries *db.Queries
	cfg     config.Config
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	if cfg.Server.Port == "" {
		log.Fatal("PORT required for api")
	}

	pool, err := pgxpool.New(context.Background(), cfg.Database.URL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	queries := db.New(pool) // sqlc-generated constructor
	app := &App{
		queries: queries,
		cfg:     cfg,
	}

	mux := http.NewServeMux()
	// 3. Configure the HTTP Server with explicit timeouts
	server := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      mux,               // Use our custom ServeMux
		ReadTimeout:  5 * time.Second,   // Time to read the entire request
		WriteTimeout: 10 * time.Second,  // Time to write the response
		IdleTimeout:  120 * time.Second, // Time to keep idle connections open
	}

	mux.HandleFunc("GET /healthz", app.handlerHealthz)

	log.Printf("Starting server on %s\n", server.Addr)
	// 4. Start the server
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("Error starting server: %v\n", err)
	}

}

func (a *App) handlerHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok\n"))
}
