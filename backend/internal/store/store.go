package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type Project struct {
	ID                int64
	PathWithNamespace string
	Name              string
	WebURL            string
	Description       sql.NullString
	DefaultBranch     sql.NullString
	StarCount         int
	ForksCount        int
	OpenIssuesCount   int
	Visibility        sql.NullString
	LastSyncedAt      sql.NullTime
	RawJSON           []byte
}

type Issue struct {
	ID              int64
	ProjectID       int64
	IID             int
	Title           string
	Description     sql.NullString
	State           string
	IssueType       sql.NullString
	WebURL          string
	AuthorUsername  sql.NullString
	AssigneesJSON   []byte
	LabelsJSON      []byte
	ModulesJSON     []byte
	MilestoneTitle  sql.NullString
	CreatedAtGitLab sql.NullTime
	UpdatedAtGitLab sql.NullTime
	ClosedAt        sql.NullTime
}

type IssueLabelEvent struct {
	ID             int64
	ProjectID      int64
	IssueID        int64
	IssueIID       int
	GitlabLabelID  sql.NullInt64
	LabelName      string
	Action         string
	AuthorUsername sql.NullString
	EventCreatedAt sql.NullTime
	RawJSON        []byte
}

func OpenMySQL(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func Migrate(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS projects (
			id BIGINT NOT NULL PRIMARY KEY,
			path_with_namespace VARCHAR(512) NOT NULL,
			name VARCHAR(512) NOT NULL,
			web_url VARCHAR(1024) NOT NULL,
			description TEXT,
			default_branch VARCHAR(255),
			star_count INT DEFAULT 0,
			forks_count INT DEFAULT 0,
			open_issues_count INT DEFAULT 0,
			visibility VARCHAR(64),
			last_synced_at DATETIME(3) NULL,
			raw_json JSON NULL,
			created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
			updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS issues (
			id BIGINT NOT NULL PRIMARY KEY,
			project_id BIGINT NOT NULL,
			iid INT NOT NULL,
			title VARCHAR(2048) NOT NULL,
			description MEDIUMTEXT,
			state VARCHAR(32) NOT NULL,
			issue_type VARCHAR(64) NULL,
			web_url VARCHAR(1024) NOT NULL,
			author_username VARCHAR(255) NULL,
			assignees_json JSON NULL,
			labels_json JSON NULL,
			modules_json JSON NULL,
			milestone_title VARCHAR(512) NULL,
			created_at_gitlab DATETIME(3) NULL,
			updated_at_gitlab DATETIME(3) NULL,
			closed_at DATETIME(3) NULL,
			synced_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
			UNIQUE KEY uq_project_iid (project_id, iid),
			KEY idx_issues_state (state),
			KEY idx_issues_project (project_id),
			CONSTRAINT fk_issues_project FOREIGN KEY (project_id) REFERENCES projects (id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS project_labels (
			project_id BIGINT NOT NULL,
			gitlab_id BIGINT NOT NULL,
			name VARCHAR(512) NOT NULL,
			color VARCHAR(32) NULL,
			text_color VARCHAR(32) NULL,
			description TEXT NULL,
			open_issues_count INT NOT NULL DEFAULT 0,
			closed_issues_count INT NOT NULL DEFAULT 0,
			work_kind VARCHAR(32) NOT NULL DEFAULT 'none',
			synced_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
			PRIMARY KEY (project_id, gitlab_id),
			KEY idx_pl_project (project_id),
			CONSTRAINT fk_pl_project FOREIGN KEY (project_id) REFERENCES projects (id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS issue_label_events (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			project_id BIGINT NOT NULL,
			issue_id BIGINT NOT NULL,
			issue_iid INT NOT NULL,
			gitlab_label_id BIGINT NULL,
			label_name VARCHAR(512) NOT NULL,
			action VARCHAR(32) NOT NULL,
			author_username VARCHAR(255) NULL,
			event_created_at DATETIME(3) NULL,
			raw_json JSON NULL,
			created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
			CONSTRAINT fk_ile_project FOREIGN KEY (project_id) REFERENCES projects (id) ON DELETE CASCADE,
			CONSTRAINT fk_ile_issue FOREIGN KEY (issue_id) REFERENCES issues (id) ON DELETE CASCADE,
			KEY idx_ile_project (project_id),
			KEY idx_ile_issue (issue_id),
			KEY idx_ile_project_iid (project_id, issue_iid),
			KEY idx_ile_event_created_at (event_created_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	// Existing DBs created before modules_json: add column if missing.
	if _, err := db.ExecContext(ctx, `ALTER TABLE issues ADD COLUMN modules_json JSON NULL`); err != nil {
		if !isMySQLDuplicateColumn(err) {
			return fmt.Errorf("migrate alter issues.modules_json: %w", err)
		}
	}
	return nil
}

func isMySQLDuplicateColumn(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column") || strings.Contains(msg, "1060")
}

func UpsertProject(ctx context.Context, db *sql.DB, p *Project) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO projects (
			id, path_with_namespace, name, web_url, description, default_branch,
			star_count, forks_count, open_issues_count, visibility, last_synced_at, raw_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			path_with_namespace = VALUES(path_with_namespace),
			name = VALUES(name),
			web_url = VALUES(web_url),
			description = VALUES(description),
			default_branch = VALUES(default_branch),
			star_count = VALUES(star_count),
			forks_count = VALUES(forks_count),
			open_issues_count = VALUES(open_issues_count),
			visibility = VALUES(visibility),
			last_synced_at = VALUES(last_synced_at),
			raw_json = VALUES(raw_json)
	`,
		p.ID, p.PathWithNamespace, p.Name, p.WebURL, nullStr(p.Description), nullStr(p.DefaultBranch),
		p.StarCount, p.ForksCount, p.OpenIssuesCount, nullStr(p.Visibility), nullTime(p.LastSyncedAt), p.RawJSON,
	)
	return err
}

func UpsertIssue(ctx context.Context, db *sql.DB, i *Issue) error {
	mj := any(nil)
	if len(i.ModulesJSON) > 0 {
		mj = i.ModulesJSON
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO issues (
			id, project_id, iid, title, description, state, issue_type, web_url,
			author_username, assignees_json, labels_json, modules_json, milestone_title,
			created_at_gitlab, updated_at_gitlab, closed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			title = VALUES(title),
			description = VALUES(description),
			state = VALUES(state),
			issue_type = VALUES(issue_type),
			web_url = VALUES(web_url),
			author_username = VALUES(author_username),
			assignees_json = VALUES(assignees_json),
			labels_json = VALUES(labels_json),
			modules_json = VALUES(modules_json),
			milestone_title = VALUES(milestone_title),
			created_at_gitlab = VALUES(created_at_gitlab),
			updated_at_gitlab = VALUES(updated_at_gitlab),
			closed_at = VALUES(closed_at)
	`,
		i.ID, i.ProjectID, i.IID, i.Title, nullStr(i.Description), i.State, nullStr(i.IssueType), i.WebURL,
		nullStr(i.AuthorUsername), i.AssigneesJSON, i.LabelsJSON, mj, nullStr(i.MilestoneTitle),
		nullTime(i.CreatedAtGitLab), nullTime(i.UpdatedAtGitLab), nullTime(i.ClosedAt),
	)
	return err
}

func InsertIssueLabelEvent(ctx context.Context, db *sql.DB, e *IssueLabelEvent) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO issue_label_events (
			project_id, issue_id, issue_iid, gitlab_label_id, label_name,
			action, author_username, event_created_at, raw_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		e.ProjectID, e.IssueID, e.IssueIID, nullInt64(e.GitlabLabelID), e.LabelName,
		e.Action, nullStr(e.AuthorUsername), nullTime(e.EventCreatedAt), e.RawJSON,
	)
	return err
}

func GetProject(ctx context.Context, db *sql.DB, id int64) (*Project, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, path_with_namespace, name, web_url, description, default_branch,
			star_count, forks_count, open_issues_count, visibility, last_synced_at, raw_json
		FROM projects WHERE id = ?
	`, id)
	var p Project
	if err := row.Scan(
		&p.ID, &p.PathWithNamespace, &p.Name, &p.WebURL, &p.Description, &p.DefaultBranch,
		&p.StarCount, &p.ForksCount, &p.OpenIssuesCount, &p.Visibility, &p.LastSyncedAt, &p.RawJSON,
	); err != nil {
		return nil, err
	}
	return &p, nil
}

type IssueRow struct {
	Issue
	Labels  []string `json:"labels"`
	Modules []string `json:"modules"`
}

// StoredModulesFromJSON returns modules when modules_json is non-empty and unmarshals to a non-empty array.
func StoredModulesFromJSON(modulesJSON []byte) ([]string, bool) {
	if len(modulesJSON) == 0 {
		return nil, false
	}
	var mods []string
	if err := json.Unmarshal(modulesJSON, &mods); err != nil || len(mods) == 0 {
		return nil, false
	}
	return dedupeModuleOrder(mods), true
}

// ModulesForIssueRow prefers persisted modules_json; otherwise derives from title/labels.
func ModulesForIssueRow(title string, labels []string, storedModulesJSON []byte) []string {
	if mods, ok := StoredModulesFromJSON(storedModulesJSON); ok {
		return mods
	}
	return IssueModules(title, labels)
}

// ListIssuesFiltered loads matching issues, applies optional module, label, and kind filters, then paginates in memory.
// catalog is required when kind is set (non-empty, not "all"); it may be nil otherwise.
func ListIssuesFiltered(ctx context.Context, db *sql.DB, projectID int64, state, module, label, kind string, catalog map[string]string, offset, limit int) ([]IssueRow, int, error) {
	where := []string{"project_id = ?"}
	args := []any{projectID}
	if state != "" && state != "all" {
		where = append(where, "state = ?")
		args = append(args, state)
	}
	w := strings.Join(where, " AND ")

	rows, err := db.QueryContext(ctx, `
		SELECT id, project_id, iid, title, description, state, issue_type, web_url,
			author_username, assignees_json, labels_json, modules_json, milestone_title,
			created_at_gitlab, updated_at_gitlab, closed_at
		FROM issues WHERE `+w+` ORDER BY updated_at_gitlab DESC`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var all []IssueRow
	for rows.Next() {
		var r IssueRow
		var mj []byte
		if err := rows.Scan(
			&r.ID, &r.ProjectID, &r.IID, &r.Title, &r.Description, &r.State, &r.IssueType, &r.WebURL,
			&r.AuthorUsername, &r.AssigneesJSON, &r.LabelsJSON, &mj, &r.MilestoneTitle,
			&r.CreatedAtGitLab, &r.UpdatedAtGitLab, &r.ClosedAt,
		); err != nil {
			return nil, 0, err
		}
		_ = json.Unmarshal(r.LabelsJSON, &r.Labels)
		r.Modules = ModulesForIssueRow(r.Title, r.Labels, mj)
		if module != "" && module != "all" {
			if !IssueHasModule(r.Modules, module) {
				continue
			}
		}
		if label != "" && label != "all" {
			if !IssueHasLabel(r.Labels, label) {
				continue
			}
		}
		if kind != "" && kind != "all" {
			got := KindFromIssueLabels(r.Labels, catalog)
			if !strings.EqualFold(strings.TrimSpace(kind), got) {
				continue
			}
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	total := len(all)
	if offset > len(all) {
		return []IssueRow{}, total, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], total, nil
}

func nullStr(ns sql.NullString) any {
	if !ns.Valid {
		return nil
	}
	return ns.String
}

func nullTime(nt sql.NullTime) any {
	if !nt.Valid {
		return nil
	}
	return nt.Time
}

func nullInt64(ni sql.NullInt64) any {
	if !ni.Valid {
		return nil
	}
	return ni.Int64
}
