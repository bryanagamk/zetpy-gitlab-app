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
	BugHeatmap         []BugHeatmapEntry  `json:"bug_heatmap"`
	ReopenStats        ReopenStats        `json:"reopen_stats"`
}

// AgingBucket is one age range for still-open issues.
type AgingBucket struct {
	RangeLabel string         `json:"range_label"`
	MinDays    int            `json:"min_days"`
	MaxDays    *int           `json:"max_days,omitempty"` // nil = no upper bound
	Total      int            `json:"total"`
	ByModule   map[string]int `json:"by_module"` // first module segment per issue (same rules as dashboard modules)
}

type BugHeatmapEntry struct {
	Module          string  `json:"module"`
	BugRatioPercent float64 `json:"bug_ratio_percent"`
	TotalIssues     int     `json:"total_issues"`
	BugCount        int     `json:"bug_count"`
}

type ReopenStats struct {
	TotalIssues         int `json:"total_issues"`
	ResolvedOnce        int `json:"resolved_once"`
	ReopenedOnce        int `json:"reopened_once"`
	ReopenedMoreThanTwo int `json:"reopened_more_than_two"`
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
		BugHeatmap:         nil,
		ReopenStats:        ReopenStats{},
	}

	// --- Open issue aging (by first activity -> In Live add; fallback to created_at_gitlab) ---
	// We compute for every issue in the project: first activity time (min event_created_at),
	// first In Live add time (min event_created_at where label_name indicates in-live and action='add').
	// If first activity is NULL we fall back to created_at_gitlab. If in-live exists, age = in_live - first_activity,
	// otherwise age = asOf - first_activity.
	rowsOpen, err := db.QueryContext(ctx, `
		SELECT i.id, i.iid, i.title, i.labels_json, i.modules_json, i.created_at_gitlab,
			MIN(e.event_created_at) AS first_activity,
			MIN(CASE WHEN (LOWER(TRIM(e.label_name)) IN ('in-live','in live','in_live','inlive')) AND LOWER(TRIM(e.action)) = 'add' THEN e.event_created_at END) AS in_live_at
		FROM issues i
		LEFT JOIN issue_label_events e ON e.issue_id = i.id
		WHERE i.project_id = ?
		GROUP BY i.id, i.iid, i.title, i.labels_json, i.modules_json, i.created_at_gitlab
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rowsOpen.Close()

	for rowsOpen.Next() {
		var (
			id            int64
			iid           int
			title         string
			labelsJSON    []byte
			modulesJSON   []byte
			created       sql.NullTime
			firstActivity sql.NullTime
			inLiveAt      sql.NullTime
		)
		if err := rowsOpen.Scan(&id, &iid, &title, &labelsJSON, &modulesJSON, &created, &firstActivity, &inLiveAt); err != nil {
			return nil, err
		}

		// determine baseline: firstActivity if present, else created
		var baseline time.Time
		if firstActivity.Valid {
			baseline = firstActivity.Time.UTC()
		} else if created.Valid {
			baseline = created.Time.UTC()
		} else {
			// nothing to base on; skip
			continue
		}

		// only include issues that have an In Live 'add' event; others are in-progress and excluded
		if !inLiveAt.Valid {
			continue
		}
		end := inLiveAt.Time.UTC()
		if end.Before(baseline) {
			continue
		}
		daysFloat := end.Sub(baseline).Hours() / 24
		if daysFloat < 0 {
			continue
		}
		ageDays := int(math.Floor(daysFloat))
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

	// --- Bug heatmap per module: compute bug ratio per module ---
	moduleTotal := map[string]int{}
	moduleBugs := map[string]int{}
	rowsMod, err := db.QueryContext(ctx, `
		SELECT title, labels_json, modules_json FROM issues WHERE project_id = ?
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rowsMod.Close()
	for rowsMod.Next() {
		var title string
		var labelsJSON []byte
		var modulesJSON []byte
		if err := rowsMod.Scan(&title, &labelsJSON, &modulesJSON); err != nil {
			return nil, err
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
		moduleTotal[mod]++
		if KindFromIssueLabels(labels, catalog) == "bug" {
			moduleBugs[mod]++
		}
	}
	if err := rowsMod.Err(); err != nil {
		return nil, err
	}
	// build heatmap entries sorted by bug ratio desc
	var heat []BugHeatmapEntry
	for m, tot := range moduleTotal {
		bugs := moduleBugs[m]
		ratio := 0.0
		if tot > 0 {
			ratio = (float64(bugs) / float64(tot)) * 100.0
		}
		heat = append(heat, BugHeatmapEntry{Module: m, BugRatioPercent: ratio, TotalIssues: tot, BugCount: bugs})
	}
	sort.Slice(heat, func(i, j int) bool { return heat[i].BugRatioPercent > heat[j].BugRatioPercent })
	out.BugHeatmap = heat

	// --- Reopened issue rate ---
	// Count reopen occurrences per issue from issue_state_events. Heuristic: count events where state contains 'reopen'.
	rowsRe, err := db.QueryContext(ctx, `
		SELECT issue_id, state, COUNT(*) as cnt FROM issue_state_events WHERE project_id = ? GROUP BY issue_id, state
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rowsRe.Close()
	reopenCounts := map[int64]int{}
	for rowsRe.Next() {
		var issueID int64
		var state string
		var cnt int
		if err := rowsRe.Scan(&issueID, &state, &cnt); err != nil {
			return nil, err
		}
		ls := strings.ToLower(strings.TrimSpace(state))
		if strings.Contains(ls, "reopen") || strings.Contains(ls, "reopened") {
			reopenCounts[issueID] += cnt
		}
	}
	if err := rowsRe.Err(); err != nil {
		return nil, err
	}
	var resolvedOnce, reopenedOnce, reopenedMore int
	// consider all issues in project
	rowsAllIssues, err := db.QueryContext(ctx, `SELECT id FROM issues WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, err
	}
	defer rowsAllIssues.Close()
	totalIssues := 0
	for rowsAllIssues.Next() {
		var iid int64
		if err := rowsAllIssues.Scan(&iid); err != nil {
			return nil, err
		}
		totalIssues++
		rc := reopenCounts[iid]
		switch {
		case rc == 0:
			resolvedOnce++
		case rc == 1:
			reopenedOnce++
		default:
			reopenedMore++
		}
	}
	out.ReopenStats = ReopenStats{
		TotalIssues:         totalIssues,
		ResolvedOnce:        resolvedOnce,
		ReopenedOnce:        reopenedOnce,
		ReopenedMoreThanTwo: reopenedMore,
	}

	// --- Resolution: GitLab closed_at and/or workflow labels in-live, close ---
	type modAgg struct {
		sumDays float64
		n       int
	}
	modMap := map[string]*modAgg{}
	var sumAll, sumBugs float64
	var nAll, nBugs int

	// --- Resolution: compute lead/resolve time using label events similar to OpenIssueAging ---
	// Start = first activity (min label event) or created_at_gitlab; End = In Live 'add' event time.
	// Only include issues that have an In Live add (i.e., Completed).
	rowsRes, err := db.QueryContext(ctx, `
		SELECT i.id, i.title, i.labels_json, i.modules_json, i.created_at_gitlab,
			MIN(e.event_created_at) AS first_activity,
			MIN(CASE WHEN (LOWER(TRIM(e.label_name)) IN ('in-live','in live','in_live','inlive')) AND LOWER(TRIM(e.action)) = 'add' THEN e.event_created_at END) AS in_live_at
		FROM issues i
		LEFT JOIN issue_label_events e ON e.issue_id = i.id
		WHERE i.project_id = ? AND i.created_at_gitlab IS NOT NULL
		GROUP BY i.id, i.title, i.labels_json, i.modules_json, i.created_at_gitlab
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rowsRes.Close()

	for rowsRes.Next() {
		var title string
		var labelsJSON []byte
		var modulesJSON []byte
		var created sql.NullTime
		var firstActivity sql.NullTime
		var inLive sql.NullTime
		if err := rowsRes.Scan(&sql.NullInt64{}, &title, &labelsJSON, &modulesJSON, &created, &firstActivity, &inLive); err != nil {
			return nil, err
		}
		var labels []string
		_ = json.Unmarshal(labelsJSON, &labels)

		// baseline
		var baseline time.Time
		if firstActivity.Valid {
			baseline = firstActivity.Time.UTC()
		} else if created.Valid {
			baseline = created.Time.UTC()
		} else {
			continue
		}

		// require inLive (completed)
		if !inLive.Valid {
			continue
		}
		end := inLive.Time.UTC()
		if end.Before(baseline) {
			continue
		}
		days := end.Sub(baseline).Hours() / 24
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
	out.ResolutionInsights.ResolutionBasis = "End = In Live 'add' event timestamp; Start = first label activity (min label event) or created_at_gitlab. Issues without an In Live add are excluded from these resolution stats."

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
