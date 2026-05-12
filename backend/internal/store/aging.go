package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// AgingIssueDetail represents an issue with computed aging duration (days) and module.
type AgingIssueDetail struct {
	ID           int64    `json:"id"`
	IID          int      `json:"iid"`
	Title        string   `json:"title"`
	Labels       []string `json:"labels"`
	Modules      []string `json:"modules"`
	Module       string   `json:"module"`
	DurationDays int      `json:"duration_days"`
	Completed    bool     `json:"completed"`
}

// ListIssuesAgingDetails returns computed aging baseline and duration per issue for a project.
func ListIssuesAgingDetails(ctx context.Context, db *sql.DB, projectID int64) ([]AgingIssueDetail, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT i.id, i.iid, i.title, i.labels_json, i.modules_json, i.created_at_gitlab,
			MIN(e.event_created_at) AS first_activity,
			MIN(CASE WHEN (LOWER(TRIM(e.label_name)) IN ('in-live','in live','in_live','inlive')) AND LOWER(TRIM(e.action)) = 'add' THEN e.event_created_at END) AS in_live_at
		FROM issues i
		LEFT JOIN issue_label_events e ON e.issue_id = i.id
		WHERE i.project_id = ?
		GROUP BY i.id, i.iid, i.title, i.labels_json, i.modules_json, i.created_at_gitlab
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list issues aging details: %w", err)
	}
	defer rows.Close()

	var out []AgingIssueDetail
	for rows.Next() {
		var (
			id            int64
			iid           int
			title         string
			labelsJSON    []byte
			modulesJSON   []byte
			created       sql.NullTime
			firstActivity sql.NullTime
			inLive        sql.NullTime
		)
		if err := rows.Scan(&id, &iid, &title, &labelsJSON, &modulesJSON, &created, &firstActivity, &inLive); err != nil {
			return nil, err
		}

		var baseline time.Time
		if firstActivity.Valid {
			baseline = firstActivity.Time.UTC()
		} else if created.Valid {
			baseline = created.Time.UTC()
		} else {
			// nothing to base on
			continue
		}

		// Only compute duration when there is an In Live 'add' event.
		completed := false
		days := 0
		if inLive.Valid {
			end := inLive.Time.UTC()
			if !end.Before(baseline) {
				days = int((end.Sub(baseline).Hours()) / 24)
				if days < 0 {
					days = 0
				}
				completed = true
			}
		}

		var labels []string
		_ = json.Unmarshal(labelsJSON, &labels)
		mods := ModulesForIssueRow(title, labels, modulesJSON)
		mod := otherModule
		if len(mods) > 0 {
			mod = strings.TrimSpace(mods[0])
			if mod == "" {
				mod = otherModule
			}
		}

		out = append(out, AgingIssueDetail{
			ID:           id,
			IID:          iid,
			Title:        title,
			Labels:       labels,
			Modules:      mods,
			Module:       mod,
			DurationDays: days,
			Completed:    completed,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
