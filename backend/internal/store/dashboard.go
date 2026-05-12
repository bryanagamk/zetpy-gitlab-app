package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
)

type Dashboard struct {
	TotalIssues    int            `json:"total_issues"`
	ByState        map[string]int `json:"by_state"`
	ByKind         map[string]int `json:"by_kind"`
	ByModule       []ModuleStat   `json:"by_module"`
	ByModuleStored []ModuleStat   `json:"by_module_stored"`
	TopLabels      []LabelStat    `json:"top_labels"`
}

type ModuleStat struct {
	Module string         `json:"module"`
	Total  int            `json:"total"`
	Opened int            `json:"opened"`
	Closed int            `json:"closed"`
	ByKind map[string]int `json:"by_kind"`
}

type LabelStat struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

func BuildDashboard(ctx context.Context, db *sql.DB, projectID int64) (*Dashboard, error) {
	catalog, err := LoadLabelKindCatalog(ctx, db, projectID)
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, `
		SELECT state, labels_json, title, modules_json FROM issues WHERE project_id = ?
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	d := &Dashboard{
		ByState: map[string]int{},
		ByKind:  map[string]int{},
	}
	modAgg := map[string]*ModuleStat{}
	modAggStored := map[string]*ModuleStat{}
	labelCounts := map[string]int{}

	for rows.Next() {
		var state string
		var labelsJSON []byte
		var title string
		var modulesJSON []byte
		if err := rows.Scan(&state, &labelsJSON, &title, &modulesJSON); err != nil {
			return nil, err
		}
		var labels []string
		_ = json.Unmarshal(labelsJSON, &labels)

		d.TotalIssues++
		d.ByState[state]++
		k := KindFromIssueLabels(labels, catalog)
		d.ByKind[k]++

		mods := ModulesForIssueRow(title, labels, modulesJSON)
		for _, mod := range mods {
			ms, ok := modAgg[mod]
			if !ok {
				ms = &ModuleStat{Module: mod, ByKind: map[string]int{}}
				modAgg[mod] = ms
			}
			ms.Total++
			ms.ByKind[k]++
			switch state {
			case "opened":
				ms.Opened++
			case "closed":
				ms.Closed++
			}
		}

		if storedMods, ok := StoredModulesFromJSON(modulesJSON); ok {
			for _, mod := range storedMods {
				ms, ok := modAggStored[mod]
				if !ok {
					ms = &ModuleStat{Module: mod, ByKind: map[string]int{}}
					modAggStored[mod] = ms
				}
				ms.Total++
				ms.ByKind[k]++
				switch state {
				case "opened":
					ms.Opened++
				case "closed":
					ms.Closed++
				}
			}
		}

		for _, l := range labels {
			labelCounts[l]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var mods []ModuleStat
	for _, m := range modAgg {
		mods = append(mods, *m)
	}
	sort.Slice(mods, func(i, j int) bool {
		if mods[i].Total == mods[j].Total {
			return mods[i].Module < mods[j].Module
		}
		return mods[i].Total > mods[j].Total
	})
	d.ByModule = mods

	var modsStored []ModuleStat
	for _, m := range modAggStored {
		modsStored = append(modsStored, *m)
	}
	sort.Slice(modsStored, func(i, j int) bool {
		if modsStored[i].Total == modsStored[j].Total {
			return modsStored[i].Module < modsStored[j].Module
		}
		return modsStored[i].Total > modsStored[j].Total
	})
	d.ByModuleStored = modsStored

	var tops []LabelStat
	for l, c := range labelCounts {
		tops = append(tops, LabelStat{Label: l, Count: c})
	}
	sort.Slice(tops, func(i, j int) bool {
		if tops[i].Count == tops[j].Count {
			return tops[i].Label < tops[j].Label
		}
		return tops[i].Count > tops[j].Count
	})
	if len(tops) > 40 {
		tops = tops[:40]
	}
	d.TopLabels = tops

	return d, nil
}

func FirstProjectID(ctx context.Context, db *sql.DB) (int64, error) {
	var id sql.NullInt64
	err := db.QueryRowContext(ctx, `SELECT id FROM projects ORDER BY id ASC LIMIT 1`).Scan(&id)
	if err != nil {
		return 0, err
	}
	if !id.Valid {
		return 0, sql.ErrNoRows
	}
	return id.Int64, nil
}
