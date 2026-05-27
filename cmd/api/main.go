package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	_ "github.com/lib/pq"
	"github.com/weilok2021/lockedin/internal/auth"
	"github.com/weilok2021/lockedin/internal/config"
	"github.com/weilok2021/lockedin/internal/database"
)

type App struct {
	db        *sql.DB
	queries   *database.Queries
	cfg       config.Config
	templates map[string]*template.Template
}

type PageData struct {
	Title   string
	Message string
	Email   string
}

// For middlewares and handlers to access to login user struct, (authorization)
type contextKey string

const userContextKey contextKey = "user"

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

	templates := make(map[string]*template.Template)
	templates["signup"] = template.Must(template.ParseFiles("web/templates/layout.html", "web/templates/signup.html"))
	templates["login"] = template.Must(template.ParseFiles("web/templates/layout.html", "web/templates/login.html"))
	templates["home"] = template.Must(template.ParseFiles("web/templates/layout.html", "web/templates/home.html"))

	app := &App{
		db:        db,
		queries:   database.New(db),
		cfg:       cfg,
		templates: templates,
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

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	mux.HandleFunc("GET /healthz", app.handlerHealthz)
	mux.HandleFunc("GET /signup", app.handlerSignUpForm)
	mux.HandleFunc("POST /signup", app.handlerSignUp)
	mux.HandleFunc("GET /login", app.handlerLoginForm)
	mux.HandleFunc("POST /login", app.handlerLogin)
	mux.HandleFunc("GET /verify", app.handlerVerifyEmail)
	mux.HandleFunc("POST /logout", app.middlewareAuthorization(app.handlerLogout))
	mux.HandleFunc("GET /", app.middlewareAuthorization(app.handlerHome))
	if cfg.Environment == "development" {
		mux.HandleFunc("POST /dev/reset", app.handlerDevReset)
	}

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

func (a *App) handlerHome(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(userContextKey).(database.User)
	a.templates["home"].ExecuteTemplate(w, "layout", PageData{
		Title: "Home",
		Email: user.Email,
	})
}

func (a *App) handlerSignUpForm(w http.ResponseWriter, r *http.Request) {
	a.templates["signup"].ExecuteTemplate(w, "layout", PageData{Title: "Sign Up"})
}

func (a *App) handlerLoginForm(w http.ResponseWriter, r *http.Request) {
	a.templates["login"].ExecuteTemplate(w, "layout", PageData{
		Title:   "Sign In",
		Message: r.URL.Query().Get("msg"),
	})
}

func (a *App) handlerSignUp(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	password := r.FormValue("password")

	if err := auth.ValidatePasswordRequirements(password); err != nil {
		responseWithError(w, 400, "Password does not meet requirements", err)
		return
	}

	hashedPassword, err := auth.HashPassword(password)
	if err != nil {
		responseWithError(w, 500, "Internal error", err)
		return
	}

	user, err := a.queries.CreateUser(r.Context(), database.CreateUserParams{
		Email:          email,
		HashedPassword: hashedPassword,
	})
	if err != nil {
		// don't reveal whether email exists — same response as success
		http.Redirect(w, r, "/login?msg=check-email", http.StatusSeeOther)
		return
	}

	token := auth.GenerateToken()
	_, err = a.queries.CreateEmailToken(r.Context(), database.CreateEmailTokenParams{
		Token:     token,
		UserID:    user.ID,
		Purpose:   "verify",
		ExpiresAt: time.Now().Add(30 * time.Minute),
	})
	if err != nil {
		responseWithError(w, 500, "Internal error", err)
		return
	}

	// dev mode: log the verify link (replace with real email later)
	log.Printf("Verify email: http://localhost:%s/verify?token=%s", a.cfg.Port, token)

	http.Redirect(w, r, "/login?msg=check-email", http.StatusSeeOther)
}

func (a *App) middlewareAuthorization(handler http.HandlerFunc) http.HandlerFunc {
	// middleware reads the cookie, looks up the session, and stashes the user in the request
	// context — then your feed pages and other protected routes can access the current user.

	return func(w http.ResponseWriter, r *http.Request) {
		sessionCookie, err := r.Cookie("session")
		// cookie not found, redirect user to login page
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		session, err := a.queries.GetSession(r.Context(), sessionCookie.Value)
		// Session not found, redirect user to login page
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		user, err := a.queries.GetUserByID(r.Context(), session.UserID)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user) // add user to context
		r = r.WithContext(ctx)

		// call handler and pass request with user added into the request's context
		handler(w, r)
	}
	// create new request with that context
}

// get request handler to handle request with the email link, if person who press this link with
// the email token, he is a verified user.
func (a *App) handlerVerifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	userID, err := a.queries.ConsumeEmailToken(r.Context(), database.ConsumeEmailTokenParams{
		Token:   token,
		Purpose: "verify",
	})
	if errors.Is(err, sql.ErrNoRows) {
		responseWithError(w, http.StatusBadRequest, "Verification link is invalid or expired", err)
		return
	}
	if err != nil {
		responseWithError(w, http.StatusInternalServerError, "Internal error", err)
		return
	}

	if err := a.queries.SetEmailVerified(r.Context(), userID); err != nil {
		responseWithError(w, 500, "Internal Email Verification Error", err)
		return
	}

	// dev mode: log the message that tell users have been redirected to login page
	log.Printf("Redirect to login page: http://localhost:%s/login", a.cfg.Port)
	http.Redirect(w, r, "/login?msg=verified", http.StatusSeeOther)
}

func (a *App) handlerLogin(w http.ResponseWriter, r *http.Request) {
	// get email, password from form
	email := r.FormValue("email")
	password := r.FormValue("password")

	// get user by email from db
	user, err := a.queries.GetUserByEmail(r.Context(), email)
	if errors.Is(err, sql.ErrNoRows) {
		responseWithError(w, http.StatusBadRequest, "User Account not available", err)
		return
	}
	if err != nil {
		responseWithError(w, http.StatusInternalServerError, "Internal error", err)
		return
	}

	// if email is not verified
	if !user.EmailVerifiedAt.Valid {
		responseWithError(w, http.StatusForbidden, "Please verify your email before logging in", nil)
		return
	}

	// compare hashed_password in user row to see if the user input password is correct.
	if err := auth.ComparePassword(user.HashedPassword, password); err != nil {
		responseWithError(w, http.StatusNotFound, "Incorrect Password", err)
		return
	}

	sessionToken := auth.GenerateToken()
	// create session for user to provide authorization for different apis/services
	session, err := a.queries.CreateSession(r.Context(), database.CreateSessionParams{
		Token:     sessionToken,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour), // the session is valid for 30 days
	})

	if err != nil {
		responseWithError(w, http.StatusInternalServerError, "Session creation error", err)
		return
	}

	// 	Server creates the cookie. When login succeeds,
	// http.SetCookie() adds a Set-Cookie header to the HTTP response.
	// The browser receives it and stores it locally.
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    session.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   30 * 24 * 60 * 60, // 30 days in seconds — match your session expiry
	})
	// dev mode: log the message that tell users have been redirected to login page
	log.Printf("Redirect to feed page: http://localhost:%s/feeds", a.cfg.Port)
	http.Redirect(w, r, "/", http.StatusSeeOther)
	// Redirect to subscribed feed page?
}

func (a *App) handlerLogout(w http.ResponseWriter, r *http.Request) {
	// 	POST /logout
	//   → DELETE FROM sessions WHERE token = <current>
	//   → Clear cookie

	// No error handling for cookie because middleware already handled it .
	sessionCookie, _ := r.Cookie("session")

	if err := a.queries.DeleteSession(r.Context(), sessionCookie.Value); err != nil {
		responseWithError(w, http.StatusInternalServerError, "Logout Failed", err)
		return
	}
	// tells the browser to delete this cookie
	http.SetCookie(w, &http.Cookie{Name: "session", Path: "/", MaxAge: -1})

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// helper function to sends json response
func responseWithJson(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	dat, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.WriteHeader(status)
	w.Write(dat)
}

// helper function to write json error
func responseWithError(w http.ResponseWriter, status int, msg string, rootCause error) {
	type errorJson struct {
		Error string `json:"error"`
	}
	resp := errorJson{
		Error: msg,
	}

	// this method will returns json
	w.Header().Set("Content-Type", "application/json")
	dat, err := json.Marshal(resp)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}

	fmt.Printf("Show the exact error: %v\n\n\n\n", rootCause)
	w.WriteHeader(status)
	w.Write(dat)
}

func (a *App) handlerDevReset(w http.ResponseWriter, r *http.Request) {
	tables := []string{"email_tokens", "sessions", "item_notifications", "user_subscriptions", "items", "feeds", "users"}
	for _, t := range tables {
		if _, err := a.db.ExecContext(r.Context(), "DELETE FROM "+t); err != nil {
			responseWithError(w, http.StatusInternalServerError, "Failed to reset "+t, err)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("all tables reset\n"))
}

// POST /signup { email, password }
//   → Validate password meets minimum requirements
//   → INSERT users (email, hashed_password = bcrypt(password), email_verified_at = NULL)
//   → INSERT email_tokens (token, user_id, purpose='verify', expires_at = now + 24h)
//   → Send verification email
//   → 200 "Check your email"
