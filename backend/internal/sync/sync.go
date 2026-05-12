package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"gitlab-api/backend/internal/gitlab"
	"gitlab-api/backend/internal/store"
)

type Service struct {
	DB     *sql.DB
	Client *gitlab.Client
	Path   string
}

func (s *Service) Run(ctx context.Context) (*store.Project, int, int, error) {
	gp, err := s.Client.GetProject(ctx, s.Path)
	if err != nil {
		return nil, 0, 0, err
	}
	raw, _ := json.Marshal(gp)

	proj := &store.Project{
		ID:                gp.ID,
		PathWithNamespace: gp.PathWithNamespace,
		Name:              gp.Name,
		WebURL:            gp.WebURL,
		Description:       sql.NullString{String: gp.Description, Valid: gp.Description != ""},
		DefaultBranch:     sql.NullString{String: gp.DefaultBranch, Valid: gp.DefaultBranch != ""},
		StarCount:         gp.StarCount,
		ForksCount:        gp.ForksCount,
		OpenIssuesCount:   gp.OpenIssuesCount,
		Visibility:        sql.NullString{String: gp.Visibility, Valid: gp.Visibility != ""},
		LastSyncedAt:      sql.NullTime{Time: time.Now().UTC(), Valid: true},
		RawJSON:           raw,
	}
	if err := store.UpsertProject(ctx, s.DB, proj); err != nil {
		return nil, 0, 0, fmt.Errorf("upsert project: %w", err)
	}

	n := 0
	err = s.Client.ListAllIssues(ctx, gp.ID, func(batch []gitlab.Issue) error {
		for _, gi := range batch {
			assignees := make([]string, 0, len(gi.Assignees))
			for _, a := range gi.Assignees {
				assignees = append(assignees, a.Username)
			}
			aj, _ := json.Marshal(assignees)
			lj, _ := json.Marshal(gi.Labels)

			var ms sql.NullString
			if gi.Milestone != nil && gi.Milestone.Title != "" {
				ms = sql.NullString{String: gi.Milestone.Title, Valid: true}
			}
			var closed sql.NullTime
			if gi.ClosedAt != nil {
				closed = sql.NullTime{Time: gi.ClosedAt.UTC(), Valid: true}
			}
			it := sql.NullString{}
			if gi.Type != "" {
				it = sql.NullString{String: gi.Type, Valid: true}
			}

			mods := store.IssueModules(gi.Title, gi.Labels)
			mj, _ := json.Marshal(mods)

			iss := &store.Issue{
				ID:              gi.ID,
				ProjectID:       gp.ID,
				IID:             gi.IID,
				Title:           gi.Title,
				Description:     sql.NullString{String: gi.Description, Valid: gi.Description != ""},
				State:           gi.State,
				IssueType:       it,
				WebURL:          gi.WebURL,
				AuthorUsername:  sql.NullString{String: gi.Author.Username, Valid: gi.Author.Username != ""},
				AssigneesJSON:   aj,
				LabelsJSON:      lj,
				ModulesJSON:     mj,
				MilestoneTitle:  ms,
				CreatedAtGitLab: sql.NullTime{Time: gi.CreatedAt.UTC(), Valid: true},
				UpdatedAtGitLab: sql.NullTime{Time: gi.UpdatedAt.UTC(), Valid: true},
				ClosedAt:        closed,
			}
			if err := store.UpsertIssue(ctx, s.DB, iss); err != nil {
				return fmt.Errorf("upsert issue %d: %w", gi.ID, err)
			}
			n++
		}
		return nil
	})
	if err != nil {
		return proj, n, 0, err
	}

	gLabels, err := s.Client.ListAllProjectLabels(ctx, gp.ID)
	if err != nil {
		return proj, n, 0, fmt.Errorf("gitlab labels: %w", err)
	}
	if err := store.ReplaceProjectLabels(ctx, s.DB, gp.ID, gLabels); err != nil {
		return proj, n, 0, fmt.Errorf("save project labels: %w", err)
	}

	proj.LastSyncedAt = sql.NullTime{Time: time.Now().UTC(), Valid: true}
	if err := store.UpsertProject(ctx, s.DB, proj); err != nil {
		return proj, n, len(gLabels), err
	}
	return proj, n, len(gLabels), nil
}
