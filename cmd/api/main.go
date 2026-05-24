package main

import (
	"database/sql"
	"log"
	"net/http"
	"time"

	_ "github.com/lib/pq"
	"github.com/weilok2021/lockedin/internal/config"
	"github.com/weilok2021/lockedin/internal/database"
)

type App struct {
	queries *database.Queries
	cfg     config.Config
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if cfg.Port == "" {
		log.Fatal("PORT required for api")
	}

	db, err := sql.Open("postgres", cfg.DbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	app := &App{
		queries: database.New(db),
		cfg:     cfg,
	}

	mux := http.NewServeMux()
	// 3. Configure the HTTP Server with explicit timeouts
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,               // Use our custom ServeMux
		ReadTimeout:  5 * time.Second,   // Time to read the entire request
		WriteTimeout: 10 * time.Second,  // Time to write the response
		IdleTimeout:  120 * time.Second, // Time to keep idle connections open
	}

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("This is the Home Page!\n"))
	})
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

// POST /signup { email, password }
//   → Validate password meets minimum requirements
//   → INSERT users (email, password_hash = bcrypt(password), email_verified_at = NULL)
//   → INSERT email_tokens (token, user_id, purpose='verify', expires_at = now + 24h)
//   → Send verification email
//   → 200 "Check your email"
