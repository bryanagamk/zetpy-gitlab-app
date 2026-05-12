package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"time"
)

type trendBucketCounts struct {
	bug, feature, improvement int
}

// IssueTrendPoint is one time bucket: issues created in GitLab during that period, by work kind.
type IssueTrendPoint struct {
	PeriodStart string `json:"period_start"` // RFC3339: week Mon UTC, month 1st UTC, or year Jan 1 UTC
	Bug         int    `json:"bug"`
	Feature     int    `json:"feature"`
	Improvement int    `json:"improvement"`
}

// IssueTrend is a time series for charting issue creation by kind.
type IssueTrend struct {
	Granularity string            `json:"granularity"`        // "week" | "month" | "year"
	ForYear     *int              `json:"for_year,omitempty"` // when set, only issues created in this calendar year (UTC)
	Points      []IssueTrendPoint `json:"points"`
}

func startOfWeekMondayUTC(t time.Time) time.Time {
	t = t.In(time.UTC)
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	w := int(d.Weekday())
	var delta int
	if w == int(time.Sunday) {
		delta = -6
	} else {
		delta = 1 - w
	}
	return d.AddDate(0, 0, delta)
}

func startOfMonthUTC(t time.Time) time.Time {
	t = t.In(time.UTC)
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func startOfYearUTC(t time.Time) time.Time {
	t = t.In(time.UTC)
	return time.Date(t.Year(), time.January, 1, 0, 0, 0, 0, time.UTC)
}

func bucketStart(createdAt time.Time, granularity string) time.Time {
	switch granularity {
	case "month":
		return startOfMonthUTC(createdAt)
	case "year":
		return startOfYearUTC(createdAt)
	default:
		return startOfWeekMondayUTC(createdAt)
	}
}

func nextBucketStart(t time.Time, granularity string) time.Time {
	switch granularity {
	case "month":
		return t.AddDate(0, 1, 0)
	case "year":
		return t.AddDate(1, 0, 0)
	default:
		return t.AddDate(0, 0, 7)
	}
}

// BuildIssueTrend counts issues by GitLab creation time and work kind (bug / feature / improvement).
// Issues without created_at_gitlab are omitted. Issues of kind "others" are not included in the three series.
// If forYear is non-nil, only issues with created_at_gitlab in [Jan 1, Jan 1 next year) UTC are included,
// and points cover every bucket in that calendar year (zeros where there were no issues).
func BuildIssueTrend(ctx context.Context, db *sql.DB, projectID int64, granularity string, forYear *int) (*IssueTrend, error) {
	if granularity != "week" && granularity != "month" && granularity != "year" {
		granularity = "week"
	}

	catalog, err := LoadLabelKindCatalog(ctx, db, projectID)
	if err != nil {
		return nil, err
	}

	var rows *sql.Rows
	if forYear != nil {
		y := *forYear
		yearStart := time.Date(y, time.January, 1, 0, 0, 0, 0, time.UTC)
		yearEnd := yearStart.AddDate(1, 0, 0)
		rows, err = db.QueryContext(ctx, `
			SELECT labels_json, created_at_gitlab
			FROM issues
			WHERE project_id = ? AND created_at_gitlab IS NOT NULL
			  AND created_at_gitlab >= ? AND created_at_gitlab < ?
		`, projectID, yearStart, yearEnd)
	} else {
		rows, err = db.QueryContext(ctx, `
			SELECT labels_json, created_at_gitlab
			FROM issues
			WHERE project_id = ? AND created_at_gitlab IS NOT NULL
		`, projectID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type counts struct {
		bug, feature, improvement int
	}
	byBucket := make(map[time.Time]*trendBucketCounts)

	for rows.Next() {
		var labelsJSON []byte
		var created sql.NullTime
		if err := rows.Scan(&labelsJSON, &created); err != nil {
			return nil, err
		}
		if !created.Valid {
			continue
		}
		var labels []string
		_ = json.Unmarshal(labelsJSON, &labels)
		k := KindFromIssueLabels(labels, catalog)
		b := bucketStart(created.Time, granularity)
		c, ok := byBucket[b]
		if !ok {
			c = &trendBucketCounts{}
			byBucket[b] = c
		}
		switch k {
		case "bug":
			c.bug++
		case "feature":
			c.feature++
		case "improvement":
			c.improvement++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := &IssueTrend{Granularity: granularity, ForYear: forYear, Points: nil}

	if forYear != nil {
		out.Points = issueTrendPointsForCalendarYear(*forYear, granularity, byBucket)
		return out, nil
	}

	if len(byBucket) == 0 {
		return out, nil
	}

	var keys []time.Time
	for k := range byBucket {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Before(keys[j]) })
	minK, maxK := keys[0], keys[len(keys)-1]

	for t := minK; !t.After(maxK); t = nextBucketStart(t, granularity) {
		c := byBucket[t]
		pt := IssueTrendPoint{PeriodStart: t.UTC().Format(time.RFC3339)}
		if c != nil {
			pt.Bug, pt.Feature, pt.Improvement = c.bug, c.feature, c.improvement
		}
		out.Points = append(out.Points, pt)
	}

	return out, nil
}

func issueTrendPointsForCalendarYear(year int, granularity string, byBucket map[time.Time]*trendBucketCounts) []IssueTrendPoint {
	yStart := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	var points []IssueTrendPoint
	appendPoint := func(t time.Time) {
		c := byBucket[t]
		pt := IssueTrendPoint{PeriodStart: t.UTC().Format(time.RFC3339)}
		if c != nil {
			pt.Bug, pt.Feature, pt.Improvement = c.bug, c.feature, c.improvement
		}
		points = append(points, pt)
	}

	switch granularity {
	case "year":
		appendPoint(startOfYearUTC(yStart))
	case "month":
		for m := time.January; m <= time.December; m++ {
			appendPoint(time.Date(year, m, 1, 0, 0, 0, 0, time.UTC))
		}
	default: // week — Mondays from the week containing Jan 1 through the week containing Dec 31
		lastDay := time.Date(year, time.December, 31, 0, 0, 0, 0, time.UTC)
		first := startOfWeekMondayUTC(yStart)
		last := startOfWeekMondayUTC(lastDay)
		for t := first; !t.After(last); t = t.AddDate(0, 0, 7) {
			appendPoint(t)
		}
	}
	return points
}
