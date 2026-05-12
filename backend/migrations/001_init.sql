CREATE TABLE IF NOT EXISTS projects (
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS issue_label_events (
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS issues (
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS project_labels (
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
