package store

import (
	"strings"
)

const uncategorized = "__Other__"

// LeadingBracketModules parses every consecutive [segment] from the start of the title.
// Example: "[Live][Shopee][Order] Fix bug" → ["Live","Shopee","Order"].
func LeadingBracketModules(title string) []string {
	s := strings.TrimSpace(title)
	var out []string
	for strings.HasPrefix(s, "[") {
		end := strings.Index(s, "]")
		if end <= 1 {
			break
		}
		inner := strings.TrimSpace(s[1:end])
		if inner != "" {
			out = append(out, inner)
		}
		s = strings.TrimSpace(s[end+1:])
	}
	return out
}

func moduleFromLabelsOnly(labels []string) string {
	for _, l := range labels {
		ll := strings.ToLower(strings.TrimSpace(l))
		for _, p := range []string{"module:", "modul:", "area:", "komponen:", "component:"} {
			if strings.HasPrefix(ll, p) {
				return strings.TrimSpace(l[len(p):])
			}
		}
	}
	for _, l := range labels {
		ll := strings.TrimSpace(l)
		if ll == "" {
			continue
		}
		lower := strings.ToLower(ll)
		if isMetaOrKindLabel(lower) {
			continue
		}
		if len(ll) >= 2 {
			return ll
		}
	}
	return uncategorized
}

// IssueModules returns taxonomy segments for an issue: all leading […] tags, or one fallback from labels.
func IssueModules(title string, labels []string) []string {
	if m := LeadingBracketModules(title); len(m) > 0 {
		return dedupeModuleOrder(m)
	}
	return []string{moduleFromLabelsOnly(labels)}
}

func dedupeModuleOrder(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, m := range in {
		m = strings.TrimSpace(m)
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	return out
}

// ModuleDisplay joins segments for a single-line summary (API field "module").
func ModuleDisplay(modules []string) string {
	if len(modules) == 0 {
		return uncategorized
	}
	return strings.Join(modules, " › ")
}

// ModuleFromIssue returns a display string; use IssueModules when you need the slice.
func ModuleFromIssue(title string, labels []string) string {
	return ModuleDisplay(IssueModules(title, labels))
}

// IssueHasModule reports whether filter matches any stored/computed segment (exact, trimmed).
func IssueHasModule(modules []string, filter string) bool {
	q := strings.TrimSpace(filter)
	if q == "" {
		return true
	}
	for _, m := range modules {
		if strings.EqualFold(strings.TrimSpace(m), q) {
			return true
		}
	}
	return false
}

// IssueHasLabel reports whether the issue includes the given label (trimmed; case-insensitive).
func IssueHasLabel(labels []string, filter string) bool {
	q := strings.TrimSpace(filter)
	if q == "" {
		return true
	}
	for _, l := range labels {
		if strings.EqualFold(strings.TrimSpace(l), q) {
			return true
		}
	}
	return false
}

// KindFromIssueLabels returns bug | feature | improvement | others.
// When catalog is non-empty (from project_labels after sync), only labels that exist in GitLab
// and are classified as bug/feature/improvement contribute; otherwise returns others.
// When catalog is empty, falls back to heuristics on issue label strings (legacy DB).
func KindFromIssueLabels(labels []string, catalog map[string]string) string {
	if len(catalog) > 0 {
		for _, l := range labels {
			key := strings.ToLower(strings.TrimSpace(l))
			switch catalog[key] {
			case "bug":
				return "bug"
			case "feature":
				return "feature"
			case "improvement":
				return "improvement"
			}
		}
		return "others"
	}
	for _, l := range labels {
		switch WorkKindForGitLabLabelName(l) {
		case "bug":
			return "bug"
		case "feature":
			return "feature"
		case "improvement":
			return "improvement"
		}
	}
	return "others"
}

// WorkKindForGitLabLabelName maps a single label name to a work kind or "none".
func WorkKindForGitLabLabelName(name string) string {
	x := strings.ToLower(strings.TrimSpace(name))
	switch {
	case containsAny(x, []string{"bug", "defect", "regression", "incident"}):
		return "bug"
	case containsAny(x, []string{"feature", "new feature", "permintaan fitur"}):
		return "feature"
	case containsAny(x, []string{"improvement", "improvements", "enhancement", "peningkatan", "optimasi", "optimization", "refactor", "debt", "technical debt", "ux", "ui polish"}):
		return "improvement"
	default:
		return "none"
	}
}

func isMetaOrKindLabel(lower string) bool {
	if WorkKindForGitLabLabelName(lower) != "none" {
		return true
	}
	return containsAny(lower, []string{
		"priority", "severity", "stage", "status", "duplicate", "blocked",
		"question", "documentation", "docs", "good first issue", "help wanted",
	})
}

func containsAny(s string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

// LabelIndicatesInLive reports whether any label marks the issue as live / shipped (workflow solved).
// Matching is case-insensitive; common variants: in-live, in live, in_live.
func LabelIndicatesInLive(labels []string) bool {
	for _, l := range labels {
		x := strings.ToLower(strings.TrimSpace(l))
		if x == "in-live" || x == "in live" || x == "in_live" || x == "inlive" {
			return true
		}
		if strings.Contains(x, "in-live") {
			return true
		}
	}
	return false
}

// LabelIndicatesWorkflowClose reports whether any label marks total workflow completion (not GitLab state).
// Matches the label name close (case-insensitive). Use a dedicated workflow label in GitLab.
func LabelIndicatesWorkflowClose(labels []string) bool {
	for _, l := range labels {
		if strings.EqualFold(strings.TrimSpace(l), "close") {
			return true
		}
	}
	return false
}
