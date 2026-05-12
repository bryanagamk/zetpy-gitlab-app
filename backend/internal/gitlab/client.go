package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *Client) get(ctx context.Context, path string, q url.Values) (*http.Response, error) {
	u := c.baseURL + "/api/v4" + path
	if enc := q.Encode(); enc != "" {
		u += "?" + enc
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	return c.httpClient.Do(req)
}

type Project struct {
	ID                int64   `json:"id"`
	PathWithNamespace string  `json:"path_with_namespace"`
	Name              string  `json:"name"`
	WebURL            string  `json:"web_url"`
	Description       string  `json:"description"`
	DefaultBranch     string  `json:"default_branch"`
	StarCount         int     `json:"star_count"`
	ForksCount        int     `json:"forks_count"`
	OpenIssuesCount   int     `json:"open_issues_count"`
	Visibility        string  `json:"visibility"`
}

func (c *Client) GetProject(ctx context.Context, pathWithNamespace string) (*Project, error) {
	enc := url.PathEscape(pathWithNamespace)
	resp, err := c.get(ctx, "/projects/"+enc, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("gitlab project: status %d: %s", resp.StatusCode, string(b))
	}
	var p Project
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

type Issue struct {
	ID          int64     `json:"id"`
	IID         int       `json:"iid"`
	ProjectID   int64     `json:"project_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	State       string    `json:"state"`
	Type        string    `json:"type"`
	WebURL      string    `json:"web_url"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at"`
	Labels      []string  `json:"labels"`
	Author      struct {
		Username string `json:"username"`
	} `json:"author"`
	Assignees []struct {
		Username string `json:"username"`
	} `json:"assignees"`
	Milestone *struct {
		Title string `json:"title"`
	} `json:"milestone"`
}

func (c *Client) ListAllIssues(ctx context.Context, projectID int64, onPage func(page []Issue) error) error {
	page := 1
	for {
		q := url.Values{}
		q.Set("state", "all")
		q.Set("scope", "all")
		q.Set("per_page", "100")
		q.Set("page", strconv.Itoa(page))
		q.Set("order_by", "updated_at")
		q.Set("sort", "desc")

		resp, err := c.get(ctx, fmt.Sprintf("/projects/%d/issues", projectID), q)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("gitlab issues page %d: status %d: %s", page, resp.StatusCode, string(body))
		}
		var batch []Issue
		if err := json.Unmarshal(body, &batch); err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}
		if err := onPage(batch); err != nil {
			return err
		}
		if len(batch) < 100 {
			break
		}
		page++
	}
	return nil
}

// ProjectLabel is one row from GET /projects/:id/labels (GitLab API v4).
// Unknown JSON fields are ignored; omit strict types for fields that vary by GitLab version.
type ProjectLabel struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	Color             string `json:"color"`
	TextColor         string `json:"text_color"`
	Description       string `json:"description"`
	OpenIssuesCount   int    `json:"open_issues_count"`
	ClosedIssuesCount int    `json:"closed_issues_count"`
}

// ListAllProjectLabels fetches every project label, paginated (per_page=100).
func (c *Client) ListAllProjectLabels(ctx context.Context, projectID int64) ([]ProjectLabel, error) {
	var all []ProjectLabel
	page := 1
	for {
		q := url.Values{}
		q.Set("per_page", "100")
		q.Set("page", strconv.Itoa(page))
		q.Set("with_counts", "true")

		resp, err := c.get(ctx, fmt.Sprintf("/projects/%d/labels", projectID), q)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("gitlab labels page %d: status %d: %s", page, resp.StatusCode, string(body))
		}
		var batch []ProjectLabel
		if err := json.Unmarshal(body, &batch); err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		all = append(all, batch...)
		if len(batch) < 100 {
			break
		}
		page++
	}
	return all, nil
}
