package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "app_http_requests_total",
		Help: "Total number of HTTP requests handled, labeled by method, route, and status code.",
	}, []string{"method", "route", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "app_http_request_duration_seconds",
		Help:    "HTTP request duration in seconds, labeled by method and route.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})

	tracer = otel.Tracer("github.com/onidemon37/tasting-journals-app")
)

//go:embed migrations/*.sql
var migrations embed.FS

//go:embed static.css
var staticFS embed.FS

type Tasting struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Distillery  string    `json:"distillery"`
	Region      string    `json:"region"`
	Country     string    `json:"country"`
	Age         *int      `json:"age,omitempty"`
	ABV         *float64  `json:"abv,omitempty"`
	CaskType    string    `json:"caskType"`
	Bottler     string    `json:"bottler"`
	Rating      *int      `json:"rating,omitempty"`
	DateTasted  time.Time `json:"dateTasted"`
	Tags        []string  `json:"tags"`
	Nose        string    `json:"nose"`
	Palate      string    `json:"palate"`
	Finish      string    `json:"finish"`
	Overall     string    `json:"overall"`
	WhatLearned string    `json:"whatLearned"`
}

func (t Tasting) ABVDisplay() string {
	if t.ABV == nil {
		return ""
	}
	return fmt.Sprintf("%.1f", *t.ABV)
}

type App struct {
	db        *pgxpool.Pool
	templates map[string]*template.Template
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx := context.Background()

	shutdownTracing, err := setupTracing(ctx)
	if err != nil {
		slog.Default().Error("tracing setup failed, continuing without it", "error", err)
	} else {
		defer shutdownTracing(ctx)
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := migrate(ctx, pool); err != nil {
		log.Fatal(err)
	}

	templates := make(map[string]*template.Template)
	appName := env("APP_NAME", "Tasting Journals")
	for _, page := range []string{"home.html", "tastings.html", "tasting.html", "new-tasting.html", "edit-tasting.html"} {
		templates[page] = template.Must(template.New(page).Funcs(template.FuncMap{"appName": func() string { return appName }}).ParseFS(templatesFS, "templates/layout.html", "templates/"+page))
	}
	app := &App{db: pool, templates: templates}
	server := &http.Server{Addr: env("APP_ADDRESS", ":8080"), Handler: app.routes(), ReadHeaderTimeout: 5 * time.Second}
	slog.Default().Info("server listening", "address", server.Addr)
	log.Fatal(server.ListenAndServe())
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	contents, err := migrations.ReadFile("migrations/001_initial.sql")
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, string(contents))
	return err
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := a.db.Ping(r.Context()); err != nil {
			writeErrorStatus(w, http.StatusServiceUnavailable, "database is not ready")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.Handle("GET /static.css", http.FileServer(http.FS(staticFS)))
	mux.HandleFunc("GET /api/tastings", a.listTastings)
	mux.HandleFunc("POST /api/tastings", a.createTasting)
	mux.HandleFunc("GET /api/tastings/{id}", a.getTasting)
	mux.HandleFunc("PUT /api/tastings/{id}", a.updateTasting)
	mux.HandleFunc("DELETE /api/tastings/{id}", a.deleteTasting)
	mux.HandleFunc("GET /tastings", a.tastingsPage)
	mux.HandleFunc("GET /tastings/new", a.newTastingPage)
	mux.HandleFunc("POST /tastings", a.createTastingFromForm)
	mux.HandleFunc("GET /tastings/{id}/edit", a.editTastingPage)
	mux.HandleFunc("POST /tastings/{id}", a.updateTastingFromForm)
	mux.HandleFunc("POST /tastings/{id}/delete", a.deleteTastingFromForm)
	mux.HandleFunc("GET /tastings/{id}", a.tastingPage)
	mux.HandleFunc("GET /", a.homePage)
	mux.Handle("GET /metrics", promhttp.Handler())
	return logging(mux)
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggingResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func logging(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = newRequestID()
		}
		_, route := mux.Handler(r)
		if route == "" {
			route = "unmatched"
		}

		ctx, span := tracer.Start(r.Context(), route, trace.WithSpanKind(trace.SpanKindServer))
		span.SetAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.route", route),
			attribute.String("http.target", r.URL.Path),
		)
		defer span.End()

		response := &loggingResponseWriter{ResponseWriter: w}
		response.Header().Set("X-Request-ID", requestID)
		mux.ServeHTTP(response, r.WithContext(context.WithValue(ctx, requestIDKey{}, requestID)))
		status := response.status
		if status == 0 {
			status = http.StatusOK
		}
		span.SetAttributes(attribute.Int("http.status_code", status))
		if status >= 500 {
			span.SetStatus(codes.Error, http.StatusText(status))
		}

		httpRequestsTotal.WithLabelValues(r.Method, route, strconv.Itoa(status)).Inc()
		httpRequestDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
		attrs := []any{
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"duration_ms", time.Since(start).Milliseconds(),
			"peer_ip", peerIP(r.RemoteAddr),
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
		}
		if spanCtx := span.SpanContext(); spanCtx.IsValid() {
			attrs = append(attrs, "trace_id", spanCtx.TraceID().String(), "span_id", spanCtx.SpanID().String())
		}
		if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
			attrs = append(attrs, "forwarded_for", forwardedFor)
		}
		if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
			attrs = append(attrs, "real_ip_header", realIP)
		}
		slog.Default().Info("http request", attrs...)
	})
}

func peerIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

type requestIDKey struct{}

func newRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "unavailable"
	}
	return fmt.Sprintf("%x", value)
}

func (a *App) homePage(w http.ResponseWriter, r *http.Request) { a.render(w, "home.html", nil) }

func (a *App) tastingsPage(w http.ResponseWriter, r *http.Request) {
	tastings, err := a.queryTastings(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.render(w, "tastings.html", map[string]any{"Tastings": tastings, "Query": r.URL.Query().Get("q")})
}

func (a *App) newTastingPage(w http.ResponseWriter, r *http.Request) {
	a.render(w, "new-tasting.html", nil)
}

func (a *App) editTastingPage(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	tasting, err := a.findTasting(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.render(w, "edit-tasting.html", tasting)
}

func (a *App) createTastingFromForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	tasting, err := tastingFromForm(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validate(tasting); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id, err := a.saveTasting(r.Context(), tasting, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/tastings/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (a *App) updateTastingFromForm(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	tasting, err := tastingFromForm(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validate(tasting); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := a.saveTasting(r.Context(), tasting, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/tastings/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (a *App) deleteTastingFromForm(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	result, err := a.db.Exec(r.Context(), "DELETE FROM tastings WHERE id = $1", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if result.RowsAffected() == 0 {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/tastings", http.StatusSeeOther)
}

func (a *App) tastingPage(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	tasting, err := a.findTasting(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.render(w, "tasting.html", tasting)
}

func (a *App) listTastings(w http.ResponseWriter, r *http.Request) {
	tastings, err := a.queryTastings(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tastings)
}

func (a *App) getTasting(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	tasting, err := a.findTasting(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErrorStatus(w, http.StatusNotFound, "tasting not found")
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tasting)
}

func (a *App) createTasting(w http.ResponseWriter, r *http.Request) {
	var input Tasting
	if err := decodeJSON(r, &input); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.Tags == nil {
		input.Tags = []string{}
	}
	if err := validate(input); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := a.saveTasting(r.Context(), input, 0)
	if err != nil {
		writeError(w, err)
		return
	}
	if input.DateTasted.IsZero() {
		input.DateTasted = time.Now().UTC()
	}
	input.ID = id
	writeJSON(w, http.StatusCreated, input)
}

func (a *App) updateTasting(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var input Tasting
	if err := decodeJSON(r, &input); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validate(input); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := a.saveTasting(r.Context(), input, id); err != nil {
		writeError(w, err)
		return
	}
	input.ID = id
	writeJSON(w, http.StatusOK, input)
}

func (a *App) deleteTasting(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := a.db.Exec(r.Context(), "DELETE FROM tastings WHERE id = $1", id)
	if err != nil {
		writeError(w, err)
		return
	}
	if result.RowsAffected() == 0 {
		writeErrorStatus(w, http.StatusNotFound, "tasting not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) queryTastings(ctx context.Context, query string) ([]Tasting, error) {
	pattern := "%" + strings.TrimSpace(query) + "%"
	rows, err := a.db.Query(ctx, `SELECT id,name,distillery,region,country,age,abv,cask_type,bottler,rating,date_tasted,tags,nose,palate,finish,overall,what_learned FROM tastings WHERE ($1 = '%%' OR name ILIKE $1 OR distillery ILIKE $1 OR region ILIKE $1 OR overall ILIKE $1 OR EXISTS (SELECT 1 FROM unnest(tags) tag WHERE tag ILIKE $1)) ORDER BY date_tasted DESC, id DESC`, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Tasting
	for rows.Next() {
		tasting, scanErr := scanTasting(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, tasting)
	}
	return result, rows.Err()
}

func (a *App) findTasting(ctx context.Context, id int64) (Tasting, error) {
	row := a.db.QueryRow(ctx, `SELECT id,name,distillery,region,country,age,abv,cask_type,bottler,rating,date_tasted,tags,nose,palate,finish,overall,what_learned FROM tastings WHERE id = $1`, id)
	return scanTasting(row)
}

type scanner interface{ Scan(...any) error }

func scanTasting(row scanner) (Tasting, error) {
	var t Tasting
	err := row.Scan(&t.ID, &t.Name, &t.Distillery, &t.Region, &t.Country, &t.Age, &t.ABV, &t.CaskType, &t.Bottler, &t.Rating, &t.DateTasted, &t.Tags, &t.Nose, &t.Palate, &t.Finish, &t.Overall, &t.WhatLearned)
	return t, err
}

func (a *App) saveTasting(ctx context.Context, t Tasting, id int64) (int64, error) {
	if t.Tags == nil {
		t.Tags = []string{}
	}
	if t.DateTasted.IsZero() {
		t.DateTasted = time.Now().UTC()
	}
	if id == 0 {
		returnID := int64(0)
		err := a.db.QueryRow(ctx, `INSERT INTO tastings (name,distillery,region,country,age,abv,cask_type,bottler,rating,date_tasted,tags,nose,palate,finish,overall,what_learned) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id`, t.Name, t.Distillery, t.Region, t.Country, t.Age, t.ABV, t.CaskType, t.Bottler, t.Rating, t.DateTasted, t.Tags, t.Nose, t.Palate, t.Finish, t.Overall, t.WhatLearned).Scan(&returnID)
		return returnID, err
	}
	result, err := a.db.Exec(ctx, `UPDATE tastings SET name=$1,distillery=$2,region=$3,country=$4,age=$5,abv=$6,cask_type=$7,bottler=$8,rating=$9,date_tasted=$10,tags=$11,nose=$12,palate=$13,finish=$14,overall=$15,what_learned=$16,updated_at=now() WHERE id=$17`, t.Name, t.Distillery, t.Region, t.Country, t.Age, t.ABV, t.CaskType, t.Bottler, t.Rating, t.DateTasted, t.Tags, t.Nose, t.Palate, t.Finish, t.Overall, t.WhatLearned, id)
	return id, errOrNotFound(result, err)
}

func errOrNotFound(result pgconnCommandTag, err error) error {
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return errors.New("tasting not found")
	}
	return nil
}

type pgconnCommandTag interface{ RowsAffected() int64 }

func (a *App) render(w http.ResponseWriter, name string, data any) {
	page, ok := a.templates[name]
	if !ok {
		http.Error(w, "page not found", http.StatusInternalServerError)
		return
	}
	if err := page.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
func pathID(r *http.Request) (int64, error) { return strconv.ParseInt(r.PathValue("id"), 10, 64) }

func tastingFromForm(r *http.Request) (Tasting, error) {
	tasting := Tasting{
		Name:        strings.TrimSpace(r.FormValue("name")),
		Distillery:  strings.TrimSpace(r.FormValue("distillery")),
		Region:      strings.TrimSpace(r.FormValue("region")),
		Country:     strings.TrimSpace(r.FormValue("country")),
		CaskType:    strings.TrimSpace(r.FormValue("caskType")),
		Bottler:     strings.TrimSpace(r.FormValue("bottler")),
		Nose:        strings.TrimSpace(r.FormValue("nose")),
		Palate:      strings.TrimSpace(r.FormValue("palate")),
		Finish:      strings.TrimSpace(r.FormValue("finish")),
		Overall:     strings.TrimSpace(r.FormValue("overall")),
		WhatLearned: strings.TrimSpace(r.FormValue("whatLearned")),
		Tags:        splitTags(r.FormValue("tags")),
	}
	for field, value := range map[string]string{"age": r.FormValue("age"), "rating": r.FormValue("rating")} {
		if value == "" {
			continue
		}
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return Tasting{}, fmt.Errorf("%s must be a number", field)
		}
		if field == "age" {
			tasting.Age = &parsed
		} else {
			tasting.Rating = &parsed
		}
	}
	if value := r.FormValue("abv"); value != "" {
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return Tasting{}, errors.New("abv must be a number")
		}
		tasting.ABV = &parsed
	}
	if value := r.FormValue("dateTasted"); value != "" {
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil {
			return Tasting{}, errors.New("date tasted must use YYYY-MM-DD")
		}
		tasting.DateTasted = parsed
	}
	return tasting, nil
}

func splitTags(value string) []string {
	var tags []string
	for _, tag := range strings.Split(value, ",") {
		if tag = strings.TrimSpace(tag); tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func validate(t Tasting) error {
	if strings.TrimSpace(t.Name) == "" {
		return errors.New("name is required")
	}
	if t.Rating != nil && (*t.Rating < 0 || *t.Rating > 100) {
		return errors.New("rating must be between 0 and 100")
	}
	if t.ABV != nil && (*t.ABV < 0 || *t.ABV > 100) {
		return errors.New("abv must be between 0 and 100")
	}
	return nil
}
func decodeJSON(r *http.Request, value any) error {
	body := io.LimitReader(r.Body, 1<<20)
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, err error) {
	writeErrorStatus(w, http.StatusInternalServerError, err.Error())
}
func writeErrorStatus(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
