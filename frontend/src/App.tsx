import type { CSSProperties, ReactNode } from "react";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Legend,
  Line,
  LineChart,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import * as api from "./api";

const kindColors: Record<string, string> = {
  bug: "#f87171",
  feature: "#60a5fa",
  improvement: "#4ade80",
  uncategorized: "#94a3b8",
};

const stateColors: Record<string, string> = {
  opened: "#fbbf24",
  closed: "#64748b",
};

function moduleStatToChartRows(m: api.ModuleStat[]) {
  return m.slice(0, 14).map((row) => ({
    name: row.module.length > 28 ? row.module.slice(0, 27) + "…" : row.module,
    full: row.module,
    bug: row.by_kind.bug ?? 0,
    feature: row.by_kind.feature ?? 0,
    improvement: row.by_kind.improvement ?? 0,
    uncategorized: row.by_kind.uncategorized ?? 0,
    total: row.total,
    opened: row.opened,
  }));
}

function ModuleStackedBar({
  data,
  height = 320,
}: {
  data: ReturnType<typeof moduleStatToChartRows>;
  height?: number;
}) {
  return (
    <div style={{ width: "100%", height }}>
      <ResponsiveContainer>
        <BarChart data={data} layout="vertical" margin={{ left: 8, right: 16 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#2a3a5c" horizontal={false} />
          <XAxis type="number" stroke="#94a3c8" />
          <YAxis type="category" dataKey="name" width={140} stroke="#94a3c8" tick={{ fontSize: 11 }} />
          <Tooltip
            contentStyle={{ background: "#0f172a", border: "1px solid var(--border)", borderRadius: 8 }}
            formatter={(value: number, name: string) => [value, name]}
            labelFormatter={(_, p) => (p?.[0]?.payload as { full?: string })?.full ?? ""}
          />
          <Legend />
          <Bar dataKey="bug" stackId="a" fill={kindColors.bug} name="Bug" />
          <Bar dataKey="feature" stackId="a" fill={kindColors.feature} name="Feature" />
          <Bar dataKey="improvement" stackId="a" fill={kindColors.improvement} name="Improvement" />
          <Bar dataKey="uncategorized" stackId="a" fill={kindColors.uncategorized} name="Uncategorized" />
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}

const agingModulePalette = ["#f87171", "#60a5fa", "#4ade80", "#fbbf24", "#a78bfa", "#22d3ee", "#fb923c", "#94a3b8"];

type AgingModuleSeries = { dataKey: string; label: string; color: string };

function buildAgingModuleChart(buckets: api.AgingBucket[]): { rows: Record<string, string | number>[]; series: AgingModuleSeries[] } {
  const counts: Record<string, number> = {};
  for (const b of buckets) {
    for (const [m, c] of Object.entries(b.by_module ?? {})) {
      counts[m] = (counts[m] ?? 0) + c;
    }
  }
  const sorted = Object.entries(counts).sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]));
  const top = sorted.slice(0, 8).map(([m]) => m);
  const series: AgingModuleSeries[] = top.map((mod, i) => ({
    dataKey: `m_${i}`,
    label: mod === "__Other__" ? "Other" : mod,
    color: agingModulePalette[i % agingModulePalette.length]!,
  }));
  const hasOther = sorted.length > top.length;
  if (hasOther) {
    series.push({ dataKey: "m_other", label: "Other modules", color: "#64748b" });
  }
  const rows = buckets.map((b) => {
    const row: Record<string, string | number> = { name: b.range_label, total: b.total };
    let sumTop = 0;
    top.forEach((mod, i) => {
      const v = b.by_module?.[mod] ?? 0;
      row[`m_${i}`] = v;
      sumTop += v;
    });
    if (hasOther) {
      row.m_other = Math.max(0, b.total - sumTop);
    }
    return row;
  });
  return { rows, series };
}

function OpenIssueAgingStackedBar({
  data,
  series,
  height = 280,
}: {
  data: Record<string, string | number>[];
  series: AgingModuleSeries[];
  height?: number;
}) {
  return (
    <div style={{ width: "100%", height }}>
      <ResponsiveContainer>
        <BarChart data={data} margin={{ left: 4, right: 12, top: 8, bottom: 4 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#2a3a5c" vertical={false} />
          <XAxis dataKey="name" stroke="#94a3c8" tick={{ fontSize: 11 }} />
          <YAxis allowDecimals={false} stroke="#94a3c8" width={40} />
          <Tooltip
            contentStyle={{ background: "#0f172a", border: "1px solid var(--border)", borderRadius: 8 }}
            formatter={(value: number, name: string) => [value, name]}
          />
          <Legend />
          {series.map((s) => (
            <Bar key={s.dataKey} dataKey={s.dataKey} stackId="age" fill={s.color} name={s.label} />
          ))}
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}

function moduleResolveToRows(stats: api.ModuleResolveStat[]) {
  return stats.map((s) => ({
    name: s.module === "__Other__" ? "Other" : s.module.length > 22 ? s.module.slice(0, 21) + "…" : s.module,
    full: s.module,
    avg: Math.round(s.avg_resolve_days * 10) / 10,
    count: s.closed_count,
  }));
}

function ModuleResolveTimeBar({
  data,
  height,
}: {
  data: ReturnType<typeof moduleResolveToRows>;
  height: number;
}) {
  return (
    <div style={{ width: "100%", height }}>
      <ResponsiveContainer>
        <BarChart data={data} layout="vertical" margin={{ left: 4, right: 20, top: 4, bottom: 4 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#2a3a5c" horizontal={false} />
          <XAxis type="number" stroke="#94a3c8" tick={{ fontSize: 11 }} domain={[0, "dataMax + 2"]} />
          <YAxis type="category" dataKey="name" width={112} stroke="#94a3c8" tick={{ fontSize: 11 }} />
          <Tooltip
            contentStyle={{ background: "#0f172a", border: "1px solid var(--border)", borderRadius: 8 }}
            formatter={(value: number) => [`${value} days`, "Avg resolve"]}
            labelFormatter={(_, p) => {
              const row = p?.[0]?.payload as { full?: string; count?: number } | undefined;
              if (!row) return "";
              return row.count != null ? `${row.full ?? ""} (${row.count} issues)` : row.full ?? "";
            }}
          />
          <Bar dataKey="avg" fill="#22d3ee" name="Avg resolve (days)" radius={[0, 4, 4, 0]} />
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}

function useAsync<T>(fn: () => Promise<T>, deps: unknown[]) {
  const [data, setData] = useState<T | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const reload = useCallback(async () => {
    setLoading(true);
    setErr(null);
    try {
      const v = await fn();
      setData(v);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
      setData(null);
    } finally {
      setLoading(false);
    }
  }, deps);
  useEffect(() => {
    void reload();
  }, [reload]);
  return { data, err, loading, reload };
}

export default function App() {
  const dash = useAsync(() => api.getDashboard(), []);
  const proj = useAsync(() => api.getProject(), []);
  const [trendGranularity, setTrendGranularity] = useState<api.IssueTrendGranularity>("week");
  const [trendForYear, setTrendForYear] = useState<number | null>(null);
  const issueTrend = useAsync(
    () => api.getIssueTrend({ granularity: trendGranularity, forYear: trendForYear }),
    [trendGranularity, trendForYear],
  );
  const workMetrics = useAsync(() => api.getWorkMetrics(), []);

  const trendYearOptions = useMemo(() => {
    const y = new Date().getUTCFullYear();
    return Array.from({ length: 20 }, (_, i) => y - i);
  }, []);

  const [syncing, setSyncing] = useState(false);
  const [syncMsg, setSyncMsg] = useState<string | null>(null);

  const onSync = async () => {
    setSyncing(true);
    setSyncMsg(null);
    try {
      const r = await api.postSync();
      setSyncMsg(`Sync OK: ${r.issues_synced} issues, ${r.labels_synced} labels.`);
      await dash.reload();
      await proj.reload();
      await issueTrend.reload();
      await workMetrics.reload();
    } catch (e) {
      setSyncMsg(e instanceof Error ? e.message : String(e));
    } finally {
      setSyncing(false);
    }
  };

  const moduleChartStored = useMemo(
    () => moduleStatToChartRows(dash.data?.by_module_stored ?? []),
    [dash.data],
  );

  const trendChartRows = useMemo(() => {
    const pts = issueTrend.data?.points ?? [];
    const g = issueTrend.data?.granularity ?? trendGranularity;
    return pts.map((p) => ({
      label: formatTrendAxisLabel(p.period_start, g),
      bug: p.bug,
      feature: p.feature,
      improvement: p.improvement,
    }));
  }, [issueTrend.data, trendGranularity]);

  const trendAllZero = useMemo(() => {
    const pts = issueTrend.data?.points;
    if (!pts || pts.length === 0) return false;
    return pts.every((p) => p.bug + p.feature + p.improvement === 0);
  }, [issueTrend.data]);

  const agingModuleChart = useMemo(
    () => buildAgingModuleChart(workMetrics.data?.open_issue_aging.buckets ?? []),
    [workMetrics.data],
  );
  const agingGrandTotal = useMemo(
    () => (workMetrics.data?.open_issue_aging.buckets ?? []).reduce((s, b) => s + b.total, 0),
    [workMetrics.data],
  );
  const resolveModuleRows = useMemo(
    () => moduleResolveToRows(workMetrics.data?.resolution.by_module ?? []),
    [workMetrics.data],
  );
  const resolveBarHeight = useMemo(() => Math.min(360, Math.max(160, 28 * resolveModuleRows.length + 40)), [resolveModuleRows.length]);

  const topModulesForChips = useMemo(() => {
    const rows = [...(dash.data?.by_module ?? [])];
    rows.sort((a, b) => b.total - a.total || a.module.localeCompare(b.module));
    return rows.slice(0, 40).map((m) => ({ module: m.module, count: m.total }));
  }, [dash.data]);

  const statePie = useMemo(() => {
    const b = dash.data?.by_state ?? {};
    return Object.entries(b).map(([name, value]) => ({ name, value }));
  }, [dash.data]);

  const [issueState, setIssueState] = useState("all");
  const [issueKind, setIssueKind] = useState("all");
  const [issueModule, setIssueModule] = useState("");
  const [issueLabel, setIssueLabel] = useState("");
  const [page, setPage] = useState(1);
  const limit = 30;

  const issues = useAsync(
    () =>
      api.getIssues({
        state: issueState,
        kind: issueKind !== "all" ? issueKind : undefined,
        module: issueModule || undefined,
        label: issueLabel || undefined,
        page,
        limit,
      }),
    [issueState, issueKind, issueModule, issueLabel, page, limit],
  );

  useEffect(() => {
    setPage(1);
  }, [issueState, issueKind, issueModule, issueLabel]);

  return (
    <div style={{ maxWidth: 1200, margin: "0 auto", padding: "28px 20px 64px" }}>
      <header
        style={{
          display: "flex",
          flexWrap: "wrap",
          gap: 16,
          alignItems: "flex-start",
          justifyContent: "space-between",
          marginBottom: 28,
        }}
      >
        <div>
          <h1 style={{ margin: 0, fontSize: "1.75rem", letterSpacing: "-0.02em" }}>zetpy-core Issues</h1>
          <p style={{ margin: "8px 0 0", color: "var(--muted)", maxWidth: 640 }}>
            GitLab issues are mirrored in MySQL. The <span className="mono">modules_json</span> column stores the module
            segment array (for example from a title like <span className="mono">[Live][Shopee][Order]</span>). The first
            chart counts only issues with segments persisted in <span className="mono">modules_json</span>; the second
            chart also includes modules inferred from the title or from <span className="mono">module:…</span> labels when
            JSON is empty. The <span className="mono">?module=…</span> filter matches each segment. Work type (bug,
            feature, …) is derived from project labels.
          </p>
        </div>
        <div style={{ display: "flex", flexDirection: "column", gap: 8, alignItems: "flex-end" }}>
          <button
            type="button"
            onClick={() => void onSync()}
            disabled={syncing}
            style={{
              padding: "10px 16px",
              borderRadius: 10,
              border: "1px solid var(--border)",
              background: "linear-gradient(135deg, rgba(124,58,237,0.35), rgba(34,211,238,0.2))",
              color: "var(--text)",
              fontWeight: 600,
            }}
          >
            {syncing ? "Syncing…" : "Sync from GitLab"}
          </button>
          {syncMsg && (
            <span style={{ fontSize: 13, color: syncMsg.startsWith("Sync OK") ? "var(--ok)" : "var(--danger)" }}>
              {syncMsg}
            </span>
          )}
        </div>
      </header>

      <section
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))",
          gap: 16,
          marginBottom: 20,
        }}
      >
        <Panel title="Repository" loading={proj.loading} error={proj.err}>
          {proj.data && (
            <div style={{ display: "grid", gap: 8, fontSize: 14 }}>
              <div>
                <span style={{ color: "var(--muted)" }}>Name</span>
                <div style={{ fontWeight: 600 }}>{proj.data.name}</div>
              </div>
              <div>
                <span style={{ color: "var(--muted)" }}>Path</span>
                <div className="mono">{proj.data.path_with_namespace}</div>
              </div>
              <div style={{ display: "flex", gap: 16, flexWrap: "wrap" }}>
                <Stat label="Stars" value={proj.data.star_count} />
                <Stat label="Fork" value={proj.data.forks_count} />
                <Stat label="Open issues (GitLab)" value={proj.data.open_issues_count} />
              </div>
              <div>
                <span style={{ color: "var(--muted)" }}>Last synced</span>
                <div>{proj.data.last_synced_at ? formatDate(proj.data.last_synced_at) : "—"}</div>
              </div>
              <a href={proj.data.web_url} target="_blank" rel="noreferrer">
                Open in GitLab
              </a>
            </div>
          )}
        </Panel>

        <Panel title="Total in database" loading={dash.loading} error={dash.err}>
          {dash.data && (
            <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
              <div style={{ fontSize: 42, fontWeight: 700, letterSpacing: "-0.04em" }}>{dash.data.total_issues}</div>
              <div style={{ display: "flex", gap: 12, flexWrap: "wrap" }}>
                {Object.entries(dash.data.by_kind).map(([k, v]) => (
                  <span
                    key={k}
                    style={{
                      padding: "4px 10px",
                      borderRadius: 999,
                      background: "var(--panel-2)",
                      border: "1px solid var(--border)",
                      fontSize: 13,
                    }}
                  >
                    <span style={{ color: kindColors[k] ?? "#cbd5e1" }}>{k}</span>
                    <span style={{ color: "var(--muted)", marginLeft: 6 }}>{v}</span>
                  </span>
                ))}
              </div>
            </div>
          )}
        </Panel>
      </section>

      <section style={{ display: "grid", gridTemplateColumns: "1fr 320px", gap: 16, marginBottom: 20 }}>
        <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
          <Panel title="Modules" loading={dash.loading} error={dash.err}>
            {dash.data && moduleChartStored.length > 0 ? (
              <ModuleStackedBar data={moduleChartStored} height={320} />
            ) : (
              !dash.loading &&
              !dash.err && (
                <div style={{ color: "var(--muted)", fontSize: 14, lineHeight: 1.5 }}>
                  No issues yet with a module array in <span className="mono">modules_json</span>. Run{" "}
                  <strong>Sync from GitLab</strong> so modules are written to the database.
                </div>
              )
            )}
          </Panel>
        </div>

        <Panel title="Issue status" loading={dash.loading} error={dash.err} tall>
          {dash.data && statePie.length > 0 ? (
            <div style={{ width: "100%", height: 320 }}>
              <ResponsiveContainer>
                <PieChart>
                  <Pie data={statePie} dataKey="value" nameKey="name" cx="50%" cy="50%" innerRadius={52} outerRadius={88} paddingAngle={2}>
                    {statePie.map((e, i) => (
                      <Cell key={i} fill={stateColors[e.name] ?? "#94a3b8"} />
                    ))}
                  </Pie>
                  <Tooltip
                    contentStyle={{ background: "#0f172a", border: "1px solid var(--border)", borderRadius: 8 }}
                  />
                  <Legend />
                </PieChart>
              </ResponsiveContainer>
            </div>
          ) : null}
        </Panel>
      </section>

      <section style={{ marginBottom: 20 }}>
        <Panel title="Issue creation trend by kind" loading={issueTrend.loading} error={issueTrend.err}>
          <>
            <p style={{ fontSize: 13, color: "var(--muted)", margin: "0 0 12px", lineHeight: 1.45 }}>
              Number of issues <strong>created in GitLab</strong> in each period, split by work kind (from project labels).
              Weeks start on Monday (UTC); months and years use the calendar period in UTC. Optionally restrict to a single
              calendar year (UTC) to see that year only. Issues without a GitLab creation time are excluded.
            </p>
            <div style={{ display: "flex", flexWrap: "wrap", gap: 12, alignItems: "center", marginBottom: 12 }}>
              <label style={{ fontSize: 13, color: "var(--muted)" }}>
                Granularity{" "}
                <select
                  value={trendGranularity}
                  onChange={(e) => setTrendGranularity(e.target.value as api.IssueTrendGranularity)}
                  style={selectStyle}
                >
                  <option value="week">Week</option>
                  <option value="month">Month</option>
                  <option value="year">Year</option>
                </select>
              </label>
              <label style={{ fontSize: 13, color: "var(--muted)" }}>
                Calendar year (UTC){" "}
                <select
                  value={trendForYear === null ? "" : String(trendForYear)}
                  onChange={(e) => {
                    const v = e.target.value;
                    setTrendForYear(v === "" ? null : Number(v));
                  }}
                  style={selectStyle}
                >
                  <option value="">All years</option>
                  {trendYearOptions.map((y) => (
                    <option key={y} value={String(y)}>
                      {y}
                    </option>
                  ))}
                </select>
              </label>
            </div>
            {trendChartRows.length > 0 ? (
              <>
                <div style={{ width: "100%", height: 320 }}>
                  <ResponsiveContainer>
                    <LineChart data={trendChartRows} margin={{ left: 4, right: 12, top: 8, bottom: 4 }}>
                      <CartesianGrid strokeDasharray="3 3" stroke="#2a3a5c" />
                      <XAxis dataKey="label" stroke="#94a3c8" tick={{ fontSize: 11 }} interval="preserveStartEnd" />
                      <YAxis allowDecimals={false} stroke="#94a3c8" width={36} />
                      <Tooltip
                        contentStyle={{ background: "#0f172a", border: "1px solid var(--border)", borderRadius: 8 }}
                      />
                      <Legend />
                      <Line type="monotone" dataKey="bug" stroke={kindColors.bug} strokeWidth={2} dot={false} name="Bug" />
                      <Line
                        type="monotone"
                        dataKey="feature"
                        stroke={kindColors.feature}
                        strokeWidth={2}
                        dot={false}
                        name="Feature"
                      />
                      <Line
                        type="monotone"
                        dataKey="improvement"
                        stroke={kindColors.improvement}
                        strokeWidth={2}
                        dot={false}
                        name="Improvement"
                      />
                    </LineChart>
                  </ResponsiveContainer>
                </div>
                {trendForYear !== null && trendAllZero ? (
                  <p style={{ fontSize: 13, color: "var(--muted)", margin: "10px 0 0", lineHeight: 1.45 }}>
                    No bug, feature, or improvement issues were created in this UTC year (or every issue was uncategorized).
                  </p>
                ) : null}
              </>
            ) : (
              !issueTrend.loading &&
              !issueTrend.err && (
                <div style={{ color: "var(--muted)", fontSize: 14 }}>
                  No trend data yet (no issues with <span className="mono">created_at_gitlab</span>). Run{" "}
                  <strong>Sync from GitLab</strong>.
                </div>
              )
            )}
            <div
              style={{
                marginTop: 16,
                padding: "12px 14px",
                borderRadius: 10,
                background: "var(--panel-2)",
                border: "1px solid var(--border)",
                fontSize: 13,
                lineHeight: 1.55,
                color: "var(--muted)",
              }}
            >
              <strong style={{ color: "var(--text)" }}>How to read this</strong>
              <ul style={{ margin: "8px 0 0", paddingLeft: 20 }}>
                <li>
                  <span style={{ color: kindColors.bug }}>Bug</span> — A sustained rise can point to release quality
                  pressure or regressions; compare with your release cadence.
                </li>
                <li>
                  <span style={{ color: kindColors.feature }}>Feature</span> — More new features over time often reflects
                  higher roadmap or market demand.
                </li>
                <li>
                  <span style={{ color: kindColors.improvement }}>Improvement</span> — Elevated counts may indicate UX
                  friction, internal workflow pain, or technical-debt cleanup work.
                </li>
              </ul>
            </div>
          </>
        </Panel>
      </section>

      <section
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(auto-fit, minmax(min(100%, 360px), 1fr))",
          gap: 16,
          marginBottom: 20,
        }}
      >
        <Panel title="Open issue age (stale backlog risk)" loading={workMetrics.loading} error={workMetrics.err}>
          {workMetrics.data && (
            <>
              <p style={{ fontSize: 13, color: "var(--muted)", margin: "0 0 12px", lineHeight: 1.45 }}>
                Open issues only, bucketed by days since <strong>GitLab creation</strong> (UTC, day-based). Stacks show
                the <strong>first module segment</strong> (same rules as the dashboard: title <span className="mono">[…]</span>{" "}
                tags and label fallbacks)—not work-kind labels. Up to eight largest modules are shown; the rest roll into{" "}
                <strong>Other modules</strong>.
              </p>
              <div style={{ fontSize: 12, color: "var(--muted)", marginBottom: 8 }}>
                Snapshot: {formatDate(workMetrics.data.open_issue_aging.as_of)}
              </div>
              {agingGrandTotal > 0 ? (
                <>
                  <OpenIssueAgingStackedBar
                    data={agingModuleChart.rows}
                    series={agingModuleChart.series}
                    height={260}
                  />
                  <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13, marginTop: 12 }}>
                    <thead>
                      <tr style={{ textAlign: "left", color: "var(--muted)" }}>
                        <th style={th}>Age range</th>
                        <th style={th}>Total</th>
                      </tr>
                    </thead>
                    <tbody>
                      {workMetrics.data.open_issue_aging.buckets.map((b) => (
                        <tr key={b.range_label} style={{ borderTop: "1px solid var(--border)" }}>
                          <td style={td}>{b.range_label}</td>
                          <td style={td}>{b.total}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </>
              ) : (
                !workMetrics.loading &&
                !workMetrics.err && (
                  <div style={{ color: "var(--muted)", fontSize: 14 }}>
                    No open issues with a GitLab creation time, or none in the database yet.
                  </div>
                )
              )}
            </>
          )}
        </Panel>

        <Panel title="Lead time & resolution" loading={workMetrics.loading} error={workMetrics.err}>
          {workMetrics.data && (
            <>
              <p style={{ fontSize: 13, color: "var(--muted)", margin: "0 0 12px", lineHeight: 1.45 }}>
                Average <strong>calendar days</strong> from GitLab <span className="mono">created</span> to an{" "}
                <strong>end time</strong> derived from your workflow: the <span className="mono">close</span> label means
                fully done; <span className="mono">in-live</span> (and variants) means solved / live; GitLab{" "}
                <span className="mono">closed</span> still counts when there is no workflow label. End time prefers{" "}
                <span className="mono">closed_at</span>, otherwise <span className="mono">updated_at</span> (approximate
                when labels moved on still-open issues). Per module uses the <strong>first</strong> module segment;
                modules with fewer than 2 qualifying issues are omitted from the chart.
              </p>
              {workMetrics.data.resolution.closed_issues_used > 0 ? (
                <>
                  <div
                    style={{
                      display: "grid",
                      gridTemplateColumns: "repeat(auto-fit, minmax(160px, 1fr))",
                      gap: 12,
                      marginBottom: 12,
                      fontSize: 14,
                    }}
                  >
                    <div style={{ padding: "10px 12px", borderRadius: 10, background: "var(--panel-2)", border: "1px solid var(--border)" }}>
                      <div style={{ color: "var(--muted)", fontSize: 12 }}>Avg resolve (all)</div>
                      <div style={{ fontWeight: 700, fontSize: 22 }}>{workMetrics.data.resolution.avg_resolve_days_all.toFixed(1)}</div>
                      <div style={{ color: "var(--muted)", fontSize: 12 }}>{workMetrics.data.resolution.closed_issues_used} issues</div>
                    </div>
                    <div style={{ padding: "10px 12px", borderRadius: 10, background: "var(--panel-2)", border: "1px solid var(--border)" }}>
                      <div style={{ color: "var(--muted)", fontSize: 12 }}>Avg resolve (bugs)</div>
                      <div style={{ fontWeight: 700, fontSize: 22, color: kindColors.bug }}>
                        {workMetrics.data.resolution.closed_bugs_used > 0
                          ? (workMetrics.data.resolution.avg_resolve_days_bugs ?? 0).toFixed(1)
                          : "—"}
                      </div>
                      <div style={{ color: "var(--muted)", fontSize: 12 }}>
                        {workMetrics.data.resolution.closed_bugs_used} bug issues
                      </div>
                    </div>
                  </div>
                  {resolveModuleRows.length > 0 ? (
                    <ModuleResolveTimeBar data={resolveModuleRows} height={resolveBarHeight} />
                  ) : (
                    <div style={{ color: "var(--muted)", fontSize: 14 }}>
                      No module has at least 2 qualifying issues for comparison (see workflow rules above).
                    </div>
                  )}
                </>
              ) : (
                !workMetrics.loading &&
                !workMetrics.err && (
                  <div style={{ color: "var(--muted)", fontSize: 14 }}>
                    No issues qualify yet: add workflow labels <span className="mono">in-live</span> or{" "}
                    <span className="mono">close</span>, close issues in GitLab, or sync so{" "}
                    <span className="mono">created_at_gitlab</span> and <span className="mono">updated_at_gitlab</span> are
                    present.
                  </div>
                )
              )}
            </>
          )}
        </Panel>
      </section>

      <section style={{ display: "grid", gap: 16, marginBottom: 20 }}>
        <Panel title="Most common modules" loading={dash.loading} error={dash.err}>
          <>
            <p style={{ fontSize: 13, color: "var(--muted)", margin: "0 0 12px", lineHeight: 1.45 }}>
              Same module aggregation as the dashboard chart (title <span className="mono">[…]</span> segments and label fallbacks).
              Click a module to filter the issue list below; click again to clear. You can still refine with the Module text field.
            </p>
            {dash.data && topModulesForChips.length > 0 ? (
              <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
                {topModulesForChips.map((t) => {
                  const active = issueModule === t.module;
                  const shown = t.module.length > 40 ? t.module.slice(0, 39) + "…" : t.module;
                  return (
                    <button
                      key={t.module}
                      type="button"
                      onClick={() => setIssueModule((prev) => (prev === t.module ? "" : t.module))}
                      style={{
                        padding: "6px 10px",
                        borderRadius: 8,
                        background: active ? "rgba(34, 211, 238, 0.12)" : "var(--panel-2)",
                        border: active ? "2px solid var(--accent-2)" : "1px solid var(--border)",
                        fontSize: 13,
                        color: "var(--text)",
                        cursor: "pointer",
                        textAlign: "left",
                        maxWidth: "100%",
                      }}
                      title={t.module}
                    >
                      <span className="mono">{shown}</span>
                      <span style={{ color: "var(--muted)", marginLeft: 8 }}>{t.count}</span>
                    </button>
                  );
                })}
              </div>
            ) : (
              !dash.loading &&
              !dash.err && (
                <div style={{ color: "var(--muted)", fontSize: 14 }}>
                  No module aggregates yet. Run <strong>Sync from GitLab</strong> after issues include title segments or module labels.
                </div>
              )
            )}
          </>
        </Panel>

        <Panel title="Most common labels" loading={dash.loading} error={dash.err}>
          <>
            <p style={{ fontSize: 13, color: "var(--muted)", margin: "0 0 12px", lineHeight: 1.45 }}>
              Click a label to filter the issue list below. Click the same label again to clear the filter.
            </p>
            {dash.data && (
              <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
                {dash.data.top_labels.map((t) => {
                  const active = issueLabel === t.label;
                  return (
                    <button
                      key={t.label}
                      type="button"
                      onClick={() => setIssueLabel((prev) => (prev === t.label ? "" : t.label))}
                      style={{
                        padding: "6px 10px",
                        borderRadius: 8,
                        background: active ? "rgba(34, 211, 238, 0.12)" : "var(--panel-2)",
                        border: active ? "2px solid var(--accent-2)" : "1px solid var(--border)",
                        fontSize: 13,
                        color: "var(--text)",
                        cursor: "pointer",
                        textAlign: "left",
                      }}
                    >
                      <span className="mono">{t.label}</span>
                      <span style={{ color: "var(--muted)", marginLeft: 8 }}>{t.count}</span>
                    </button>
                  );
                })}
              </div>
            )}
          </>
        </Panel>
      </section>

      <section style={{ marginTop: 28 }}>
        <h2 style={{ fontSize: "1.15rem", margin: "0 0 12px" }}>Issue list</h2>
        <div style={{ display: "flex", flexWrap: "wrap", gap: 12, marginBottom: 12, alignItems: "center" }}>
          <label style={{ fontSize: 13, color: "var(--muted)" }}>
            Status{" "}
            <select
              value={issueState}
              onChange={(e) => setIssueState(e.target.value)}
              style={selectStyle}
            >
              <option value="all">All</option>
              <option value="opened">Opened</option>
              <option value="closed">Closed</option>
            </select>
          </label>
          <label style={{ fontSize: 13, color: "var(--muted)" }}>
            Kind{" "}
            <select value={issueKind} onChange={(e) => setIssueKind(e.target.value)} style={selectStyle}>
              <option value="all">All</option>
              <option value="bug">Bug</option>
              <option value="feature">Feature</option>
              <option value="improvement">Improvement</option>
              <option value="uncategorized">Uncategorized</option>
            </select>
          </label>
          <label style={{ fontSize: 13, color: "var(--muted)" }}>
            Module{" "}
            <input
              value={issueModule}
              onChange={(e) => setIssueModule(e.target.value)}
              placeholder="leave empty = all"
              style={{ ...selectStyle, minWidth: 200 }}
            />
          </label>
          {issueLabel ? (
            <span style={{ fontSize: 13, color: "var(--muted)" }}>
              Label filter: <span className="mono">{issueLabel}</span>{" "}
              <button type="button" onClick={() => setIssueLabel("")} style={ghostBtn}>
                Clear label
              </button>
            </span>
          ) : null}
          {issueModule ? (
            <span style={{ fontSize: 13, color: "var(--muted)" }}>
              Module filter: <span className="mono">{issueModule}</span>{" "}
              <button type="button" onClick={() => setIssueModule("")} style={ghostBtn}>
                Clear module
              </button>
            </span>
          ) : null}
          <button type="button" onClick={() => void issues.reload()} style={ghostBtn}>
            Reload table
          </button>
        </div>

        <Panel title="" loading={issues.loading} error={issues.err}>
          {issues.data && (
            <>
              <div style={{ overflowX: "auto" }}>
                <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
                  <thead>
                    <tr style={{ textAlign: "left", color: "var(--muted)" }}>
                      <th style={th}>IID</th>
                      <th style={th}>Title</th>
                      <th style={th}>Module (segments)</th>
                      <th style={th}>Kind</th>
                      <th style={th}>Status</th>
                    </tr>
                  </thead>
                  <tbody>
                    {issues.data.items.map((it) => (
                      <tr key={it.id} style={{ borderTop: "1px solid var(--border)" }}>
                        <td style={td} className="mono">
                          {it.iid}
                        </td>
                        <td style={td}>
                          <a href={it.web_url} target="_blank" rel="noreferrer">
                            {it.title}
                          </a>
                        </td>
                        <td style={td}>
                          {(it.modules?.length ? it.modules : [it.module]).map((m) => (
                            <span
                              key={m}
                              style={{
                                display: "inline-block",
                                margin: "0 6px 4px 0",
                                padding: "2px 8px",
                                borderRadius: 6,
                                background: "var(--panel-2)",
                                border: "1px solid var(--border)",
                                fontSize: 12,
                              }}
                            >
                              {m}
                            </span>
                          ))}
                        </td>
                        <td style={td}>
                          <span style={{ color: kindColors[it.kind] ?? "#cbd5e1" }}>{it.kind}</span>
                        </td>
                        <td style={td}>{it.state}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginTop: 12 }}>
                <span style={{ color: "var(--muted)", fontSize: 13 }}>
                  Total {issues.data.total} — page {issues.data.page}
                </span>
                <div style={{ display: "flex", gap: 8 }}>
                  <button
                    type="button"
                    disabled={page <= 1}
                    onClick={() => setPage((p) => Math.max(1, p - 1))}
                    style={ghostBtn}
                  >
                    Previous
                  </button>
                  <button
                    type="button"
                    disabled={page * limit >= issues.data.total}
                    onClick={() => setPage((p) => p + 1)}
                    style={ghostBtn}
                  >
                    Next
                  </button>
                </div>
              </div>
            </>
          )}
        </Panel>
      </section>
    </div>
  );
}

function Panel({
  title,
  children,
  loading,
  error,
  tall,
}: {
  title: string;
  children: ReactNode;
  loading?: boolean;
  error?: string | null;
  tall?: boolean;
}) {
  return (
    <div
      style={{
        background: "var(--panel)",
        border: "1px solid var(--border)",
        borderRadius: 14,
        padding: 16,
        minHeight: tall ? 380 : undefined,
      }}
    >
      {title ? (
        <h2 style={{ margin: "0 0 12px", fontSize: "1rem", color: "var(--muted)", fontWeight: 600 }}>{title}</h2>
      ) : null}
      {loading && <div style={{ color: "var(--muted)" }}>Loading…</div>}
      {error && <div style={{ color: "var(--danger)" }}>{error}</div>}
      {!loading && !error && children}
    </div>
  );
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div>
      <div style={{ color: "var(--muted)", fontSize: 12 }}>{label}</div>
      <div style={{ fontWeight: 700, fontSize: 18 }}>{value}</div>
    </div>
  );
}

function formatDate(iso: string) {
  try {
    return new Date(iso).toLocaleString("en-US");
  } catch {
    return iso;
  }
}

function formatTrendAxisLabel(periodStartISO: string, granularity: api.IssueTrendGranularity) {
  try {
    const d = new Date(periodStartISO);
    if (granularity === "year") {
      return d.toLocaleDateString("en-US", { year: "numeric", timeZone: "UTC" });
    }
    if (granularity === "month") {
      return d.toLocaleDateString("en-US", { month: "short", year: "numeric", timeZone: "UTC" });
    }
    return d.toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric", timeZone: "UTC" });
  } catch {
    return periodStartISO;
  }
}

const selectStyle: CSSProperties = {
  marginLeft: 8,
  padding: "6px 10px",
  borderRadius: 8,
  border: "1px solid var(--border)",
  background: "var(--panel-2)",
  color: "var(--text)",
};

const ghostBtn: CSSProperties = {
  padding: "8px 12px",
  borderRadius: 8,
  border: "1px solid var(--border)",
  background: "transparent",
  color: "var(--text)",
};

const th: CSSProperties = { padding: "8px 6px", fontWeight: 600 };
const td: CSSProperties = { padding: "10px 6px", verticalAlign: "top" };
