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
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/weilok2021/lockedin/internal/auth"
	"github.com/weilok2021/lockedin/internal/config"
	"github.com/weilok2021/lockedin/internal/database"
	"github.com/weilok2021/lockedin/internal/feeds"
	"github.com/weilok2021/lockedin/internal/fetcher"
)

type App struct {
	db        *sql.DB
	queries   *database.Queries
	cfg       config.Config
	templates map[string]*template.Template
}

type CatalogCard struct {
	Feed        database.Feed
	IsFollowing bool
}

type PageData struct {
	Title         string
	Message       string
	Email         string
	Subscriptions []database.ListUserSubscriptionsRow
	Catalog       []CatalogCard // ← add this
	Items         []database.ListItemsForUserRow
	Pagination    Pagination
}

type Pagination struct {
	Page        int   // current page, 1-based
	TotalPages  int   // total number of pages
	PageNumbers []int // [1, 2, 3, …] — for the clickable page links
	FirstItem   int   // 1-based index of the first item shown (e.g. 21)
	LastItem    int   // 1-based index of the last item shown  (e.g. 40)
	TotalItems  int   // total items across all your subscriptions (e.g. 87)
	HasPrevPage bool  // (page > 1)
	HasNextPage bool  // page < TotalPages
	PrevPage    int   // page - 1
	NextPage    int   // page + 1
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
	templates["subscriptions"] = template.Must(template.ParseFiles("web/templates/layout.html", "web/templates/subscriptions.html"))
	templates["catalog"] = template.Must(template.ParseFiles("web/templates/layout.html", "web/templates/catalog.html"))
	templates["landing"] = template.Must(template.ParseFiles("web/templates/layout.html", "web/templates/landing.html"))

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
	mux.HandleFunc("GET /", app.handlerRoot)

	// USER subscriptions
	mux.HandleFunc("GET /subscriptions", app.middlewareAuthorization(app.handlerListSubscriptions))
	mux.HandleFunc("POST /subscriptions/{feed_id}", app.middlewareAuthorization(app.handlerCreateSubscription))
	mux.HandleFunc("POST /subscriptions/{feed_id}/delete", app.middlewareAuthorization(app.handlerDeleteSubscription))

	// catalogs
	mux.HandleFunc("GET /catalog", app.middlewareAuthorization(app.handlerBrowseCatalog))

	// search topic/discover
	mux.HandleFunc("POST /discover", app.middlewareAuthorization(app.handlerDiscoverTopic))

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

// handlerRoot serves the public landing page to logged-out visitors and the
// personalized feed to logged-in users (spec §6.4). It does not redirect.
func (a *App) handlerRoot(w http.ResponseWriter, r *http.Request) {
	// no session: show the public landing page (do NOT redirect to /login)
	user, ok := a.userFromSession(r)
	if !ok {
		a.templates["landing"].ExecuteTemplate(w, "layout", PageData{Title: "LockedIn"})
		return
	}
	// logged in: stash the user in context and hand off to the feed handler
	ctx := context.WithValue(r.Context(), userContextKey, user)
	a.handlerHome(w, r.WithContext(ctx))
}

// userFromSession resolves the session cookie to a user without redirecting.
// ok is false when there is no valid session (cookie missing, session or user not found).
func (a *App) userFromSession(r *http.Request) (database.User, bool) {
	// cookie -> session -> user; any miss returns ok=false instead of redirecting
	sessionCookie, err := r.Cookie("session")
	if err != nil {
		return database.User{}, false
	}
	session, err := a.queries.GetSession(r.Context(), sessionCookie.Value)
	if err != nil {
		return database.User{}, false
	}
	user, err := a.queries.GetUserByID(r.Context(), session.UserID)
	if err != nil {
		return database.User{}, false
	}
	return user, true
}

// handlerHome renders the reading feed: a paginated list (20 per page) of
// items from the sources the logged-in user follows, newest first. It reads
// ?page from the query, clamping out-of-range values into [1, totalPages].
// Reached via handlerRoot ("GET /") when a valid session exists.
func (a *App) handlerHome(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(userContextKey).(database.User)
	const pageSize = 20

	page := 1
	if n, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && n > 1 {
		page = n
	}

	// 1. Ask the DB how many items exist. Everything else is arithmetic.
	total, err := a.queries.CountItemsForUser(r.Context(), user.ID)
	if err != nil {
		responseWithError(w, http.StatusInternalServerError, "Could not load your feed", err)
		return
	}

	totalPages := (int(total) + pageSize - 1) / pageSize // ceil division
	if totalPages < 1 {
		totalPages = 1 // always "Page 1 of 1", even with zero items
	}
	if page > totalPages {
		page = totalPages // clamp ?page=999 back to the last real page
	}

	// 2. Fetch exactly this page — Limit IS the page size now. No +1 probe.
	offset := (page - 1) * pageSize
	items, err := a.queries.ListItemsForUser(r.Context(), database.ListItemsForUserParams{
		UserID: user.ID,
		Limit:  pageSize,
		Offset: int32(offset),
	})
	if err != nil {
		responseWithError(w, http.StatusInternalServerError, "Could not load your feed", err)
		return
	}

	// 3. Build the clickable page numbers [1..totalPages].
	pageNumbers := make([]int, totalPages)
	for i := range pageNumbers {
		pageNumbers[i] = i + 1
	}

	firstItem := offset + 1
	if total == 0 {
		firstItem = 0 // nothing to show
	}

	a.templates["home"].ExecuteTemplate(w, "layout", PageData{
		Title: "Home",
		Email: user.Email,
		Items: items,
		Pagination: Pagination{
			Page:        page,
			TotalPages:  totalPages,
			PageNumbers: pageNumbers,
			FirstItem:   firstItem,
			LastItem:    offset + len(items),
			TotalItems:  int(total),
			HasNextPage: page < totalPages,
			HasPrevPage: page > 1,
			PrevPage:    page - 1,
			NextPage:    page + 1,
		},
	})
}

// isHTTPURL reports whether s is a plain http(s) URL — safe for <img src>.
func isHTTPURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
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

// GET /sources lists the user's subscriptions showing the topic label, the source, and last fetch status.
func (a *App) handlerListSubscriptions(w http.ResponseWriter, r *http.Request) {
	// pull the user the middleware put into this request, and treat it as a database.User.
	user := r.Context().Value(userContextKey).(database.User)
	subscriptions, err := a.queries.ListUserSubscriptions(r.Context(), user.ID)
	if err != nil {
		responseWithError(w, http.StatusInternalServerError, "Could not load subscriptions", err)
		return
	}
	a.templates["subscriptions"].ExecuteTemplate(w, "layout", PageData{
		Title:         "Subscriptions",
		Email:         user.Email,
		Message:       r.URL.Query().Get("msg"),
		Subscriptions: subscriptions,
	})
}

// handlerCreateSubscription (POST /subscriptions):
//  1. userから context; topic := r.FormValue("topic")
//  2. feedURL, _, err := feeds.FeedURLForTopic(topic)   // err → redirect ?msg=invalid
//  3. validate: fetch + parse feedURL with gofeed        // err → redirect ?msg=invalid
//  4. feedRow := UpsertFeed(...)  → CreateUserSubscription(user.ID, feedRow.ID, topic)
//  5. redirect /subscriptions?msg=added

// POST /sources accepts a topic string, constructs the provider feed URL,
// validates it's a real feed (one-shot fetch + parse),
// upserts the feeds row, inserts user_subscriptions with the topic as custom_title.
// Returns a clear error if the fetch/parse fails.

// func (a *App) handlerCreateSubscription(w http.ResponseWriter, r *http.Request) {
// 	topic := r.FormValue("topic")
// 	feedURL, _, err := feeds.FeedURLForTopic(topic)
// 	if err != nil {
// 		http.Redirect(w, r, "/subscriptions?msg=invalid", http.StatusSeeOther)
// 		return
// 	}
// 	fp := gofeed.NewParser()
// 	fp.UserAgent = "Lockedin/0.1 (+https://github.com/weilok2021/lockedin)"

// 	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
// 	defer cancel()

// 	fetchedFeed, err := fp.ParseURLWithContext(feedURL, ctx)
// 	if err != nil {
// 		http.Redirect(w, r, "/subscriptions?msg=invalid", http.StatusSeeOther)
// 		return
// 	}
// 	dbFeed, err := a.queries.UpsertFeed(r.Context(), database.UpsertFeedParams{
// 		FeedUrl: feedURL,
// 		Title:   sql.NullString{String: fetchedFeed.Title, Valid: fetchedFeed.Title != ""},
// 		SiteUrl: sql.NullString{String: fetchedFeed.Link, Valid: fetchedFeed.Link != ""},
// 	})
// 	if err != nil {
// 		responseWithError(w, http.StatusInternalServerError, "Could not save subscription", err)
// 		return
// 	}
// 	user := r.Context().Value(userContextKey).(database.User)
// 	if err := a.queries.CreateUserSubscription(r.Context(), database.CreateUserSubscriptionParams{
// 		UserID:      user.ID,
// 		FeedID:      dbFeed.ID,
// 		CustomTitle: sql.NullString{String: topic, Valid: true},
// 	}); err != nil {
// 		responseWithError(w, http.StatusInternalServerError, "Could not save subscription", err)
// 		return
// 	}
// 	http.Redirect(w, r, "/subscriptions?msg=added", http.StatusSeeOther)
// }

func (a *App) handlerCreateSubscription(w http.ResponseWriter, r *http.Request) {
	feedID, err := uuid.Parse(r.PathValue("feed_id"))
	if err != nil {
		http.Redirect(w, r, "/catalog?msg=invalid", http.StatusSeeOther)
		return
	}

	user := r.Context().Value(userContextKey).(database.User)
	if err := a.queries.CreateUserSubscription(r.Context(), database.CreateUserSubscriptionParams{
		UserID: user.ID,
		FeedID: feedID,
	}); err != nil {
		responseWithError(w, http.StatusInternalServerError, "Could not save subscription", err)
		return
	}
	http.Redirect(w, r, "/catalog?msg=added", http.StatusSeeOther)
}

func (a *App) handlerDeleteSubscription(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(userContextKey).(database.User)
	feedID, err := uuid.Parse(r.PathValue("feed_id"))
	if err != nil {
		http.Redirect(w, r, "/subscriptions?msg=invalid", http.StatusSeeOther)
		return
	}
	if err := a.queries.DeleteUserSubscription(r.Context(), database.DeleteUserSubscriptionParams{
		UserID: user.ID,
		FeedID: feedID,
	}); err != nil {
		responseWithError(w, http.StatusInternalServerError, "Could not remove subscription", err)
		return
	}
	http.Redirect(w, r, "/subscriptions?msg=removed", http.StatusSeeOther)
}

func (a *App) handlerBrowseCatalog(w http.ResponseWriter, r *http.Request) {
	catalogs := []CatalogCard{}
	user := r.Context().Value(userContextKey).(database.User)
	feeds, err := a.queries.ListCatalog(r.Context())
	if err != nil {
		responseWithError(w, http.StatusInternalServerError, "Failed to list catalog", err)
		return
	}

	feedIDs, err := a.queries.ListFollowedFeedIDs(r.Context(), user.ID)
	if err != nil {
		responseWithError(w, http.StatusInternalServerError, "Failed to list user followed feeds", err)
		return
	}

	// Add all feeds into catalog
	for _, feed := range feeds {
		catalogs = append(catalogs, CatalogCard{
			Feed:        feed,
			IsFollowing: false,
		})
	}

	// Update feeds in catalog to 'followed' if it is followed by user
	for i, card := range catalogs {
		for _, feedID := range feedIDs {
			if card.Feed.ID == feedID {
				catalogs[i].IsFollowing = true
			}
		}
	}

	a.templates["catalog"].ExecuteTemplate(w, "layout", PageData{
		Title:   "Catalog",
		Email:   user.Email,
		Message: r.URL.Query().Get("msg"),
		Catalog: catalogs,
	})
}

func (a *App) handlerDiscoverTopic(w http.ResponseWriter, r *http.Request) {
	topic := r.FormValue("topic")
	user := r.Context().Value(userContextKey).(database.User)

	feedURL, _, err := feeds.FeedURLForTopic(topic)
	if err != nil {
		http.Redirect(w, r, "/catalog?msg=invalid", http.StatusSeeOther)
		return
	}

	fetchedFeed, err := fetcher.FetchFeed(r.Context(), feedURL)
	if err != nil {
		http.Redirect(w, r, "/catalog?msg=invalid", http.StatusSeeOther)
		return
	}
	dbFeed, err := a.queries.UpsertFeed(r.Context(), database.UpsertFeedParams{
		FeedUrl: feedURL,
		Title:   sql.NullString{String: fetchedFeed.Title, Valid: fetchedFeed.Title != ""},
		SiteUrl: sql.NullString{String: fetchedFeed.Link, Valid: fetchedFeed.Link != ""},
	})
	if err != nil {
		responseWithError(w, http.StatusInternalServerError, "Could not save subscription", err)
		return
	}

	if err := a.queries.CreateUserSubscription(r.Context(), database.CreateUserSubscriptionParams{
		UserID:      user.ID,
		FeedID:      dbFeed.ID,
		CustomTitle: sql.NullString{String: topic, Valid: true},
	}); err != nil {
		responseWithError(w, http.StatusInternalServerError, "Could not save subscription", err)
		return
	}

	fetcher.StoreFeedItems(r.Context(), a.queries, fetchedFeed, dbFeed.ID)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) handlerShowMoreItems(w http.ResponseWriter, r *http.Request) {
	//   1. InsertItem → :execrows so it returns rows-affected (1 = inserted, 0 = deduped), then sqlc generate.
	//   2. ListSubscribedFeeds query → returns id + feed_url for the user's followed feeds (the fetch loop's input; ListUserSubscriptions omits feed_url).
	//   3. StoreFeedItems → change to return (int, error), summing the new-insert counts. (The two existing callers discard the return, so they keep compiling.)
	//   4. handlerRefresh (rename your boilerplate) → ListSubscribedFeeds → per feed FetchFeed + StoreFeedItems with per-feed log-and-continue → redirect to
	//   /?fetched=<total> (303).
	//   5. Route → POST /refresh behind middlewareAuthorization.
	//   6. handlerHome → read ?fetched (use r.URL.Query().Has("fetched") to tell "no fetch" from "0 new") and pass it into PageData.
	return
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
	tables := []string{"email_tokens", "sessions", "item_notifications", "user_subscriptions", "items", "feeds"}
	for _, t := range tables {
		if _, err := a.db.ExecContext(r.Context(), "DELETE FROM "+t); err != nil {
			responseWithError(w, http.StatusInternalServerError, "Failed to reset "+t, err)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("all tables reset except users table\n"))
}
