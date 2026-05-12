package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"gitlab-api/backend/internal/config"
	"gitlab-api/backend/internal/gitlab"
	"gitlab-api/backend/internal/store"
	"gitlab-api/backend/internal/sync"
)

type Server struct {
	Cfg *config.Config
	DB  *sql.DB
}

func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) PostSync(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()

	svc := &sync.Service{
		DB:     s.DB,
		Client: gitlab.New(s.Cfg.GitLabBaseURL, s.Cfg.GitLabAccessToken),
		Path:   s.Cfg.GitLabProjectPath,
	}
	proj, n, nLabels, err := svc.Run(ctx)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project_id":     proj.ID,
		"issues_synced":  n,
		"labels_synced":  nLabels,
		"last_synced_at": proj.LastSyncedAt.Time,
	})
}

func (s *Server) GetProject(w http.ResponseWriter, r *http.Request) {
	id, err := store.FirstProjectID(r.Context(), s.DB)
	if err != nil {
		if err == sql.ErrNoRows {
			writeErr(w, http.StatusNotFound, "no project data yet; run POST /api/sync")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	p, err := store.GetProject(r.Context(), s.DB, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":                  p.ID,
		"path_with_namespace": p.PathWithNamespace,
		"name":                p.Name,
		"web_url":             p.WebURL,
		"description":         nullString(p.Description),
		"default_branch":      nullString(p.DefaultBranch),
		"star_count":          p.StarCount,
		"forks_count":         p.ForksCount,
		"open_issues_count":   p.OpenIssuesCount,
		"visibility":          nullString(p.Visibility),
		"last_synced_at":      nullTime(p.LastSyncedAt),
	})
}

func (s *Server) GetLabels(w http.ResponseWriter, r *http.Request) {
	id, err := store.FirstProjectID(r.Context(), s.DB)
	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusOK, []store.ProjectLabelRow{})
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	labels, err := store.ListProjectLabels(r.Context(), s.DB, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, labels)
}

func (s *Server) GetDashboard(w http.ResponseWriter, r *http.Request) {
	id, err := store.FirstProjectID(r.Context(), s.DB)
	if err != nil {
		if err == sql.ErrNoRows {
			writeErr(w, http.StatusNotFound, "no data yet; run POST /api/sync")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	d, err := store.BuildDashboard(r.Context(), s.DB, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) GetIssueTrend(w http.ResponseWriter, r *http.Request) {
	id, err := store.FirstProjectID(r.Context(), s.DB)
	if err != nil {
		if err == sql.ErrNoRows {
			writeErr(w, http.StatusNotFound, "no data yet; run POST /api/sync")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	g := r.URL.Query().Get("granularity")
	if g != "week" && g != "month" && g != "year" {
		g = "week"
	}
	var forYear *int
	if fy := r.URL.Query().Get("for_year"); fy != "" {
		if y, err := strconv.Atoi(fy); err == nil && y >= 1990 && y <= 2100 {
			forYear = &y
		}
	}
	tr, err := store.BuildIssueTrend(r.Context(), s.DB, id, g, forYear)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tr)
}

func (s *Server) GetWorkMetrics(w http.ResponseWriter, r *http.Request) {
	id, err := store.FirstProjectID(r.Context(), s.DB)
	if err != nil {
		if err == sql.ErrNoRows {
			writeErr(w, http.StatusNotFound, "no data yet; run POST /api/sync")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	m, err := store.BuildWorkMetrics(r.Context(), s.DB, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) GetOpenAgingIssues(w http.ResponseWriter, r *http.Request) {
	id, err := store.FirstProjectID(r.Context(), s.DB)
	if err != nil {
		if err == sql.ErrNoRows {
			writeErr(w, http.StatusNotFound, "no data yet; run POST /api/sync")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	bucketStr := r.URL.Query().Get("bucket")
	// bucket can be index (0..3) or range label; prefer index
	bucket := -1
	if bucketStr != "" {
		if b, err := strconv.Atoi(bucketStr); err == nil {
			bucket = b
		}
	}
	details, err := store.ListIssuesAgingDetails(r.Context(), s.DB, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// if bucket specified, filter
	if bucket >= 0 {
		// compute bucket index locally (same logic as work_metrics.ageBucketIndex)
		filtered := make([]AgingIssueDTO, 0, len(details))
		for _, d := range details {
			// skip in-progress issues (no In Live add)
			if !d.Completed {
				continue
			}
			bi := func(ageDays int) int {
				switch {
				case ageDays < 7:
					return 0
				case ageDays < 30:
					return 1
				case ageDays < 91:
					return 2
				default:
					return 3
				}
			}(d.DurationDays)
			if bi == bucket {
				filtered = append(filtered, AgingIssueDTOFromDetail(d))
			}
		}
		// sort by duration desc for modal presentation
		sort.Slice(filtered, func(i, j int) bool { return filtered[i].DurationDays > filtered[j].DurationDays })
		writeJSON(w, http.StatusOK, map[string]any{"items": filtered, "total": len(filtered)})
		return
	}
	// otherwise return all
	out := make([]AgingIssueDTO, 0, len(details))
	for _, d := range details {
		out = append(out, AgingIssueDTOFromDetail(d))
	}
	// sort all results by duration desc to provide consistent ordering
	sort.Slice(out, func(i, j int) bool { return out[i].DurationDays > out[j].DurationDays })
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "total": len(out)})
}

type AgingIssueDTO struct {
	ID           int64    `json:"id"`
	IID          int      `json:"iid"`
	Title        string   `json:"title"`
	Labels       []string `json:"labels"`
	Modules      []string `json:"modules"`
	Module       string   `json:"module"`
	DurationDays int      `json:"duration_days"`
	Completed    bool     `json:"completed"`
}

func AgingIssueDTOFromDetail(d store.AgingIssueDetail) AgingIssueDTO {
	return AgingIssueDTO{
		ID:           d.ID,
		IID:          d.IID,
		Title:        d.Title,
		Labels:       d.Labels,
		Modules:      d.Modules,
		Module:       d.Module,
		DurationDays: d.DurationDays,
		Completed:    d.Completed,
	}
}

type issueDTO struct {
	ID              int64    `json:"id"`
	IID             int      `json:"iid"`
	Title           string   `json:"title"`
	State           string   `json:"state"`
	WebURL          string   `json:"web_url"`
	Author          string   `json:"author"`
	Labels          []string `json:"labels"`
	Modules         []string `json:"modules"`
	Module          string   `json:"module"`
	Kind            string   `json:"kind"`
	Milestone       string   `json:"milestone,omitempty"`
	CreatedAtGitLab *string  `json:"created_at_gitlab,omitempty"`
	UpdatedAtGitLab *string  `json:"updated_at_gitlab,omitempty"`
}

func (s *Server) GetIssues(w http.ResponseWriter, r *http.Request) {
	id, err := store.FirstProjectID(r.Context(), s.DB)
	if err != nil {
		if err == sql.ErrNoRows {
			writeErr(w, http.StatusNotFound, "no data yet; run POST /api/sync")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	state := r.URL.Query().Get("state")
	if state == "" {
		state = "all"
	}
	module := r.URL.Query().Get("module")
	label := r.URL.Query().Get("label")
	kind := r.URL.Query().Get("kind")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 200 {
		limit = 50
	}
	offset := (page - 1) * limit

	catalog, err := store.LoadLabelKindCatalog(r.Context(), s.DB, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	rows, total, err := store.ListIssuesFiltered(r.Context(), s.DB, id, state, module, label, kind, catalog, offset, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]issueDTO, 0, len(rows))
	for _, row := range rows {
		d := issueDTO{
			ID:      row.ID,
			IID:     row.IID,
			Title:   row.Title,
			State:   row.State,
			WebURL:  row.WebURL,
			Labels:  row.Labels,
			Modules: row.Modules,
			Module:  store.ModuleDisplay(row.Modules),
			Kind:    store.KindFromIssueLabels(row.Labels, catalog),
		}
		if row.AuthorUsername.Valid {
			d.Author = row.AuthorUsername.String
		}
		if row.MilestoneTitle.Valid {
			d.Milestone = row.MilestoneTitle.String
		}
		if row.CreatedAtGitLab.Valid {
			t := row.CreatedAtGitLab.Time.UTC().Format(time.RFC3339)
			d.CreatedAtGitLab = &t
		}
		if row.UpdatedAtGitLab.Valid {
			t := row.UpdatedAtGitLab.Time.UTC().Format(time.RFC3339)
			d.UpdatedAtGitLab = &t
		}
		out = append(out, d)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": out,
		"page":  page,
		"limit": limit,
		"total": total,
	})
}

func (s *Server) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/health", s.Health)
	r.Route("/api", func(r chi.Router) {
		r.Post("/sync", s.PostSync)
		r.Get("/project", s.GetProject)
		r.Get("/dashboard", s.GetDashboard)
		r.Get("/dashboard/issue-trend", s.GetIssueTrend)
		r.Get("/dashboard/work-metrics", s.GetWorkMetrics)
		r.Get("/labels", s.GetLabels)
		r.Get("/issues", s.GetIssues)
		r.Get("/dashboard/open-aging/issues", s.GetOpenAgingIssues)
	})
	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func nullString(ns sql.NullString) any {
	if !ns.Valid {
		return nil
	}
	return ns.String
}

func nullTime(nt sql.NullTime) any {
	if !nt.Valid {
		return nil
	}
	return nt.Time.UTC().Format(time.RFC3339)
}
