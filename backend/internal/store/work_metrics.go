package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"
)

const otherModule = "__Other__"

// WorkMetrics bundles backlog aging and closed-issue resolution stats.
type WorkMetrics struct {
	OpenIssueAging     OpenIssueAging     `json:"open_issue_aging"`
	ResolutionInsights ResolutionInsights `json:"resolution"`
}

// AgingBucket is one age range for still-open issues.
type AgingBucket struct {
	RangeLabel string         `json:"range_label"`
	MinDays    int            `json:"min_days"`
	MaxDays    *int           `json:"max_days,omitempty"` // nil = no upper bound
	Total      int            `json:"total"`
	ByModule   map[string]int `json:"by_module"` // first module segment per issue (same rules as dashboard modules)
}

// OpenIssueAging counts opened issues by age since GitLab creation (UTC day-based).
type OpenIssueAging struct {
	AsOfRFC3339 string        `json:"as_of"`
	Buckets     []AgingBucket `json:"buckets"`
}

// ModuleResolveStat is average lead time for issues attributed to a module.
type ModuleResolveStat struct {
	Module         string  `json:"module"`
	AvgResolveDays float64 `json:"avg_resolve_days"`
	ClosedCount    int     `json:"closed_count"` // issues in the resolution sample (see resolution_basis)
}

// ResolutionInsights summarizes lead / resolution time using GitLab dates plus workflow labels.
type ResolutionInsights struct {
	AvgResolveDaysAll  float64             `json:"avg_resolve_days_all"`
	AvgResolveDaysBugs *float64            `json:"avg_resolve_days_bugs,omitempty"`
	ClosedIssuesUsed   int                 `json:"closed_issues_used"` // count of issues in the averages
	ClosedBugsUsed     int                 `json:"closed_bugs_used"`
	ResolutionBasis    string              `json:"resolution_basis"`
	ByModule           []ModuleResolveStat `json:"by_module"`
}

func ageBucketIndex(ageDays int) int {
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
}

func openAgingBucketDefs() []AgingBucket {
	max7 := 7
	max30 := 30
	max90 := 90
	return []AgingBucket{
		{RangeLabel: "0–7 days", MinDays: 0, MaxDays: &max7, ByModule: map[string]int{}},
		{RangeLabel: "7–30 days", MinDays: 7, MaxDays: &max30, ByModule: map[string]int{}},
		{RangeLabel: "30–90 days", MinDays: 30, MaxDays: &max90, ByModule: map[string]int{}},
		{RangeLabel: ">90 days", MinDays: 91, MaxDays: nil, ByModule: map[string]int{}},
	}
}

// BuildWorkMetrics computes open-issue aging and closed-issue resolution metrics.
func BuildWorkMetrics(ctx context.Context, db *sql.DB, projectID int64) (*WorkMetrics, error) {
	catalog, err := LoadLabelKindCatalog(ctx, db, projectID)
	if err != nil {
		return nil, err
	}

	asOf := time.Now().UTC()
	out := &WorkMetrics{
		OpenIssueAging: OpenIssueAging{
			AsOfRFC3339: asOf.Format(time.RFC3339),
			Buckets:     openAgingBucketDefs(),
		},
		ResolutionInsights: ResolutionInsights{ByModule: nil},
	}

	// --- Open issue aging (by first module segment; not by work-kind labels) ---
	rowsOpen, err := db.QueryContext(ctx, `
		SELECT title, labels_json, modules_json, created_at_gitlab
		FROM issues
		WHERE project_id = ? AND state = 'opened' AND created_at_gitlab IS NOT NULL
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rowsOpen.Close()

	for rowsOpen.Next() {
		var title string
		var labelsJSON, modulesJSON []byte
		var created sql.NullTime
		if err := rowsOpen.Scan(&title, &labelsJSON, &modulesJSON, &created); err != nil {
			return nil, err
		}
		if !created.Valid {
			continue
		}
		ageHours := asOf.Sub(created.Time.UTC()).Hours()
		ageDays := int(math.Floor(ageHours / 24))
		if ageDays < 0 {
			ageDays = 0
		}
		bi := ageBucketIndex(ageDays)
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
		out.OpenIssueAging.Buckets[bi].Total++
		out.OpenIssueAging.Buckets[bi].ByModule[mod]++
	}
	if err := rowsOpen.Err(); err != nil {
		return nil, err
	}

	// --- Resolution: GitLab closed_at and/or workflow labels in-live, close ---
	type modAgg struct {
		sumDays float64
		n       int
	}
	modMap := map[string]*modAgg{}
	var sumAll, sumBugs float64
	var nAll, nBugs int

	rowsRes, err := db.QueryContext(ctx, `
		SELECT labels_json, title, modules_json, state, created_at_gitlab, closed_at, updated_at_gitlab
		FROM issues
		WHERE project_id = ? AND created_at_gitlab IS NOT NULL AND updated_at_gitlab IS NOT NULL
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rowsRes.Close()

	for rowsRes.Next() {
		var labelsJSON []byte
		var title, state string
		var modulesJSON []byte
		var created, closed, updated sql.NullTime
		if err := rowsRes.Scan(&labelsJSON, &title, &modulesJSON, &state, &created, &closed, &updated); err != nil {
			return nil, err
		}
		var labels []string
		_ = json.Unmarshal(labelsJSON, &labels)
		end, ok := effectiveResolutionEnd(state, labels, created, closed, updated)
		if !ok {
			continue
		}
		days := end.Sub(created.Time.UTC()).Hours() / 24
		if days < 0 {
			continue
		}

		nAll++
		sumAll += days
		k := KindFromIssueLabels(labels, catalog)
		if k == "bug" {
			nBugs++
			sumBugs += days
		}

		mods := ModulesForIssueRow(title, labels, modulesJSON)
		mod := otherModule
		if len(mods) > 0 {
			mod = mods[0]
		}
		if mod == "" {
			mod = otherModule
		}
		ma, okM := modMap[mod]
		if !okM {
			ma = &modAgg{}
			modMap[mod] = ma
		}
		ma.sumDays += days
		ma.n++
	}
	if err := rowsRes.Err(); err != nil {
		return nil, err
	}

	out.ResolutionInsights.ClosedIssuesUsed = nAll
	out.ResolutionInsights.ClosedBugsUsed = nBugs
	if nAll > 0 {
		out.ResolutionInsights.AvgResolveDaysAll = sumAll / float64(nAll)
	}
	if nBugs > 0 {
		avg := sumBugs / float64(nBugs)
		out.ResolutionInsights.AvgResolveDaysBugs = &avg
	}

	const minModuleIssues = 2
	var modStats []ModuleResolveStat
	for mod, a := range modMap {
		if a.n < minModuleIssues {
			continue
		}
		modStats = append(modStats, ModuleResolveStat{
			Module:         mod,
			AvgResolveDays: a.sumDays / float64(a.n),
			ClosedCount:    a.n,
		})
	}
	sort.Slice(modStats, func(i, j int) bool {
		if modStats[i].AvgResolveDays == modStats[j].AvgResolveDays {
			return modStats[i].Module < modStats[j].Module
		}
		return modStats[i].AvgResolveDays > modStats[j].AvgResolveDays
	})
	if len(modStats) > 15 {
		modStats = modStats[:15]
	}
	out.ResolutionInsights.ByModule = modStats
	out.ResolutionInsights.ResolutionBasis = "End = GitLab closed_at when available; otherwise GitLab updated_at when the issue qualifies: workflow label close (fully done), GitLab state closed, or workflow in-live (solved / live). Exact label-change times are not stored; updated_at approximates the last move for open issues."

	return out, nil
}

func effectiveResolutionEnd(state string, labels []string, created, closed, updated sql.NullTime) (time.Time, bool) {
	if !created.Valid || !updated.Valid {
		return time.Time{}, false
	}
	c := created.Time.UTC()
	u := updated.Time.UTC()
	if u.Before(c) {
		return time.Time{}, false
	}
	hasClose := LabelIndicatesWorkflowClose(labels)
	hasLive := LabelIndicatesInLive(labels)
	isClosed := strings.EqualFold(strings.TrimSpace(state), "closed")
	closedOK := closed.Valid && !closed.Time.UTC().Before(c)

	switch {
	case hasClose:
		if closedOK {
			return closed.Time.UTC(), true
		}
		return u, true
	case isClosed:
		if closedOK {
			return closed.Time.UTC(), true
		}
		return u, true
	case hasLive:
		if closedOK {
			return closed.Time.UTC(), true
		}
		return u, true
	default:
		return time.Time{}, false
	}
}
