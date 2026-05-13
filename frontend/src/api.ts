const prefix = (import.meta.env.VITE_API_BASE_URL ?? "").replace(/\/$/, "");

async function parseJSON<T>(res: Response): Promise<T> {
  const text = await res.text();
  if (!res.ok) {
    let msg = res.statusText;
    try {
      const j = JSON.parse(text) as { error?: string };
      if (j.error) msg = j.error;
    } catch {
      if (text) msg = text;
    }
    throw new Error(msg);
  }
  return JSON.parse(text) as T;
}

export type ModuleStat = {
  module: string;
  total: number;
  opened: number;
  closed: number;
  by_kind: Record<string, number>;
};

export type Dashboard = {
  total_issues: number;
  by_state: Record<string, number>;
  by_kind: Record<string, number>;
  by_module: ModuleStat[];
  /** Aggregate for issues that have a non-empty modules_json column in the DB */
  by_module_stored?: ModuleStat[];
  top_labels: { label: string; count: number }[];
};

export type IssueTrendGranularity = "week" | "month" | "year";

export type IssueTrendPoint = {
  period_start: string;
  bug: number;
  feature: number;
  improvement: number;
};

export type IssueTrend = {
  granularity: IssueTrendGranularity;
  /** When present, data is limited to issues created in this calendar year (UTC). */
  for_year?: number;
  points: IssueTrendPoint[];
};

export type AgingBucket = {
  range_label: string;
  min_days: number;
  max_days?: number;
  total: number;
  by_module: Record<string, number>;
};

export type OpenIssueAging = {
  as_of: string;
  buckets: AgingBucket[];
};

export type ModuleResolveStat = {
  module: string;
  avg_resolve_days: number;
  closed_count: number;
};

export type ResolutionInsights = {
  avg_resolve_days_all: number;
  avg_resolve_days_bugs?: number;
  closed_issues_used: number;
  closed_bugs_used: number;
  resolution_basis?: string;
  by_module: ModuleResolveStat[];
};

export type WorkMetrics = {
  open_issue_aging: OpenIssueAging;
  resolution: ResolutionInsights;
};

export type BugHeatmapEntry = {
  module: string;
  bug_ratio_percent: number;
  total_issues: number;
  bug_count: number;
};

export type ReopenStats = {
  total_issues: number;
  resolved_once: number;
  reopened_once: number;
  reopened_more_than_two: number;
};

// extend WorkMetrics with new visuals
export type WorkMetricsExtended = WorkMetrics & {
  bug_heatmap: BugHeatmapEntry[];
  reopen_stats: ReopenStats;
};

export type Project = {
  id: number;
  path_with_namespace: string;
  name: string;
  web_url: string;
  description: string | null;
  default_branch: string | null;
  star_count: number;
  forks_count: number;
  open_issues_count: number;
  visibility: string | null;
  last_synced_at: string | null;
};

export type IssueRow = {
  id: number;
  iid: number;
  title: string;
  state: string;
  web_url: string;
  author: string;
  labels: string[];
  /** Segments from leading [A][B][C] in the title, or a single fallback from labels */
  modules: string[];
  /** Single-line combined display value */
  module: string;
  kind: string;
  milestone?: string;
  created_at_gitlab?: string;
  updated_at_gitlab?: string;
};

export async function getDashboard(): Promise<Dashboard> {
  const res = await fetch(`${prefix}/api/dashboard`);
  return parseJSON<Dashboard>(res);
}

export async function getIssueTrend(params: {
  granularity: IssueTrendGranularity;
  /** If set, only issues created in this calendar year (UTC). Omit for all time. */
  forYear?: number | null;
}): Promise<IssueTrend> {
  const q = new URLSearchParams({ granularity: params.granularity });
  if (params.forYear != null && Number.isFinite(params.forYear)) {
    q.set("for_year", String(params.forYear));
  }
  const res = await fetch(`${prefix}/api/dashboard/issue-trend?${q.toString()}`);
  return parseJSON<IssueTrend>(res);
}

export async function getWorkMetrics(): Promise<WorkMetricsExtended> {
  const res = await fetch(`${prefix}/api/dashboard/work-metrics`);
  return parseJSON<WorkMetricsExtended>(res);
}

export async function getProject(): Promise<Project> {
  const res = await fetch(`${prefix}/api/project`);
  return parseJSON<Project>(res);
}

export async function postSync(): Promise<{
  project_id: number;
  issues_synced: number;
  labels_synced: number;
  last_synced_at: string;
}> {
  const res = await fetch(`${prefix}/api/sync`, { method: "POST" });
  return parseJSON<{
    project_id: number;
    issues_synced: number;
    labels_synced: number;
    last_synced_at: string;
  }>(res);
}

export function streamSync(onEvent: (data: any) => void): EventSource {
  const es = new EventSource(`${prefix}/api/sync/stream`);
  es.onmessage = (e) => {
    try {
      onEvent(JSON.parse(e.data));
    } catch (err) {
      // ignore malformed
    }
  };
  es.onerror = () => {
    // EventSource will reconnect automatically; consumers may close it.
  };
  return es;
}

export async function postClientMetrics(metrics: any): Promise<{ ok: boolean }> {
  const res = await fetch(`${prefix}/api/client-metrics`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(metrics),
  });
  return parseJSON<{ ok: boolean }>(res);
}

export async function getIssues(params: {
  state?: string;
  module?: string;
  label?: string;
  kind?: string;
  page?: number;
  limit?: number;
}): Promise<{ items: IssueRow[]; total: number; page: number; limit: number }> {
  const q = new URLSearchParams();
  if (params.state) q.set("state", params.state);
  if (params.module) q.set("module", params.module);
  if (params.label) q.set("label", params.label);
  if (params.kind) q.set("kind", params.kind);
  if (params.page) q.set("page", String(params.page));
  if (params.limit) q.set("limit", String(params.limit));
  const res = await fetch(`${prefix}/api/issues?${q.toString()}`);
  return parseJSON(res);
}

export async function getAgingBucketIssues(bucket: number): Promise<{ items: { id: number; iid: number; title: string; labels: string[]; modules: string[]; module: string; duration_days: number }[]; total: number }> {
  const q = new URLSearchParams();
  q.set("bucket", String(bucket));
  const res = await fetch(`${prefix}/api/dashboard/open-aging/issues?${q.toString()}`);
  return parseJSON(res);
}

export async function getReopenedIssues(category: string): Promise<{ items: { id: number; iid: number; title: string; labels: string[]; modules: string[]; module: string; reopen_count: number }[]; total: number }> {
  const q = new URLSearchParams();
  q.set("category", category);
  const res = await fetch(`${prefix}/api/dashboard/reopened/issues?${q.toString()}`);
  return parseJSON(res);
}
