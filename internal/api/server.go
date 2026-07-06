package api

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	"github.com/NPetersenDK/eBot-CSGO-API/internal/docs"
)

// Server holds shared handler dependencies.
type Server struct {
	db *sql.DB
}

// New builds the HTTP router. apiKey guards every route except health, the
// OpenAPI spec and the Swagger UI.
func New(db *sql.DB, apiKey string) http.Handler {
	s := &Server{db: db}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Public: liveness + API docs.
	r.Get("/health", s.health)
	r.Get("/openapi.json", docs.ServeSpec)
	r.Get("/swagger/*", httpSwagger.Handler(httpSwagger.URL("/openapi.json")))

	// Authenticated API.
	r.Group(func(r chi.Router) {
		r.Use(apiKeyAuth(apiKey))

		r.Route("/matches", func(r chi.Router) {
			r.Get("/", s.listMatches)
			r.Post("/", s.createMatch)
			r.Get("/{id}", s.getMatch)
			r.Delete("/{id}", s.deleteMatch)
			r.Post("/{id}/start", s.startMatch)
			r.Post("/{id}/archive", s.archiveMatch)

			// Granular match data (populated by the bot as the match plays).
			r.Get("/{id}/maps", s.listMatchMaps)
			r.Get("/{id}/players", s.listMatchPlayers)
			r.Get("/{id}/rounds", s.listMatchRounds)
			r.Get("/{id}/kills", s.listMatchKills)
		})

		r.Route("/servers", func(r chi.Router) {
			r.Get("/", s.listServers)
			r.Post("/", s.createServer)
			r.Get("/{id}", s.getServer)
			r.Put("/{id}", s.updateServer)
			r.Delete("/{id}", s.deleteServer)
		})

		r.Get("/teams", s.listTeams)
		r.Get("/seasons", s.listSeasons)
	})

	return r
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	status := "ok"
	code := http.StatusOK
	if err := s.db.PingContext(r.Context()); err != nil {
		status, code = "db unreachable", http.StatusServiceUnavailable
	}
	writeJSON(w, code, map[string]string{"status": status})
}

// dbError logs the underlying error and returns a generic 500 to the client.
func (s *Server) dbError(w http.ResponseWriter, err error) {
	log.Printf("db error: %v", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}
