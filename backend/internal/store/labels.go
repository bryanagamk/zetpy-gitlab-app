package store

import (
	"context"
	"database/sql"
	"strings"

	"gitlab-api/backend/internal/gitlab"
)

// ProjectLabelRow is one row in project_labels (API JSON shape).
type ProjectLabelRow struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	Color             string `json:"color"`
	TextColor         string `json:"text_color"`
	Description       string `json:"description,omitempty"`
	OpenIssuesCount   int    `json:"open_issues_count"`
	ClosedIssuesCount int    `json:"closed_issues_count"`
	WorkKind          string `json:"work_kind"`
}

// ReplaceProjectLabels replaces all labels for a project (full sync from GitLab).
func ReplaceProjectLabels(ctx context.Context, db *sql.DB, projectID int64, labels []gitlab.ProjectLabel) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM project_labels WHERE project_id = ?`, projectID); err != nil {
		return err
	}

	const ins = `INSERT INTO project_labels (
		project_id, gitlab_id, name, color, text_color, description,
		open_issues_count, closed_issues_count, work_kind
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	for _, lb := range labels {
		wk := WorkKindForGitLabLabelName(lb.Name)
		desc := strings.TrimSpace(lb.Description)
		var descArg any
		if desc != "" {
			descArg = desc
		}
		_, err = tx.ExecContext(ctx, ins,
			projectID, lb.ID, strings.TrimSpace(lb.Name),
			nullIfEmpty(lb.Color), nullIfEmpty(lb.TextColor), descArg,
			lb.OpenIssuesCount, lb.ClosedIssuesCount, wk,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func nullIfEmpty(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}

// ListProjectLabels returns stored GitLab labels for the project (newest sync order by name).
func ListProjectLabels(ctx context.Context, db *sql.DB, projectID int64) ([]ProjectLabelRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT gitlab_id, name, color, text_color, description,
			open_issues_count, closed_issues_count, work_kind
		FROM project_labels WHERE project_id = ? ORDER BY name ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProjectLabelRow
	for rows.Next() {
		var r ProjectLabelRow
		var color, textColor, desc sql.NullString
		if err := rows.Scan(&r.ID, &r.Name, &color, &textColor, &desc,
			&r.OpenIssuesCount, &r.ClosedIssuesCount, &r.WorkKind); err != nil {
			return nil, err
		}
		if color.Valid {
			r.Color = color.String
		}
		if textColor.Valid {
			r.TextColor = textColor.String
		}
		if desc.Valid {
			r.Description = desc.String
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// LoadLabelKindCatalog maps lower(trim(label name)) -> work_kind for bug | feature | improvement only.
func LoadLabelKindCatalog(ctx context.Context, db *sql.DB, projectID int64) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT LOWER(TRIM(name)), work_kind FROM project_labels WHERE project_id = ?
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[string]string)
	for rows.Next() {
		var nameKey, wk string
		if err := rows.Scan(&nameKey, &wk); err != nil {
			return nil, err
		}
		switch wk {
		case "bug", "feature", "improvement":
			m[nameKey] = wk
		default:
			// "none" and unknown: do not map — issue labels that only match these are skipped for kind
		}
	}
	return m, rows.Err()
}
