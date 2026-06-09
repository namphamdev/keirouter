import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  Cpu,
  MemoryStick,
  HardDrive,
  Zap,
  Server,
  AlertTriangle,
  RefreshCw,
  Globe,
} from "lucide-react";
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  ResponsiveContainer,
  Tooltip,
  CartesianGrid,
} from "recharts";
import { api } from "../lib/api";
import { PageHeader } from "../components/Layout";
import { Card, SectionHeader, StatCard, Spinner, ErrorCard, Badge } from "../components/ui";

// ---- helpers ----------------------------------------------------------------

function formatUptime(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (h < 24) return `${h}h ${m}m`;
  const d = Math.floor(h / 24);
  return `${d}d ${h % 24}h ${m}m`;
}

function pctColor(pct: number): "accent" | "warning" | "danger" {
  if (pct >= 85) return "danger";
  if (pct >= 60) return "warning";
  return "accent";
}

function pctTone(pct: number): "accent" | "warning" | "danger" {
  return pctColor(pct);
}

function tsLabel(ts: number): string {
  const d = new Date(ts * 1000);
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

// ---- page -------------------------------------------------------------------

export function SystemPage() {
  const snap = useQuery({
    queryKey: ["system"],
    queryFn: () => api.systemMonitor(),
    refetchInterval: 5000,
  });

  const history = useQuery({
    queryKey: ["system-history"],
    queryFn: () => api.systemHistory(),
    refetchInterval: 5000,
  });

  if (snap.isLoading) return <Spinner />;
  if (snap.isError) return <ErrorCard message={(snap.error as Error).message} />;

  const s = snap.data!;
  const h = history.data;

  const chartData = (h?.samples ?? []).map((pt) => ({
    time: tsLabel(pt.ts),
    cpu: +pt.cpu_pct.toFixed(1),
    mem: +pt.mem_pct.toFixed(1),
    goroutines: pt.goroutines,
    heap: +pt.heap_mb.toFixed(1),
  }));

  const spikes = h?.spikes ?? [];

  return (
    <>
      <PageHeader
        title="System Monitor"
        description="Real-time resource usage and runtime health"
        icon={Activity}
        action={
          <button
            onClick={() => { snap.refetch(); history.refetch(); }}
            className="inline-flex items-center gap-1.5 rounded-xl border border-[var(--border)] bg-[var(--bg-elevated)] px-3 py-2 text-sm font-medium text-[var(--text-muted)] transition-colors hover:bg-ink-100 dark:hover:bg-ink-800"
          >
            <RefreshCw className="h-3.5 w-3.5" />
            Refresh
          </button>
        }
      />

      {/* ── Stat cards ───────────────────────────────────────────── */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4 mb-6">
        <StatCard
          icon={Cpu}
          iconTone={pctTone(s.cpu_pct)}
          label="CPU"
          value={`${s.cpu_pct.toFixed(1)}%`}
        />
        <StatCard
          icon={MemoryStick}
          iconTone={pctTone(s.mem_pct)}
          label="Memory"
          value={`${s.mem_pct.toFixed(1)}%`}
          delta={{ text: `${s.mem_used_mb} / ${s.mem_total_mb} MB` }}
        />
        <StatCard
          icon={HardDrive}
          iconTone={pctTone(s.disk_pct)}
          label="Disk"
          value={`${s.disk_pct.toFixed(1)}%`}
          delta={{ text: `${s.disk_used_gb.toFixed(1)} / ${s.disk_total_gb.toFixed(1)} GB` }}
        />
        <StatCard
          icon={Zap}
          iconTone="accent"
          label="Goroutines"
          value={s.goroutines.toLocaleString()}
        />
      </div>

      {/* ── Charts ───────────────────────────────────────────────── */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 mb-6">
        <MetricChart
          title="CPU Usage"
          description="System CPU percentage over time"
          icon={Cpu}
          data={chartData}
          dataKey="cpu"
          color="var(--color-accent-500)"
          unit="%"
          threshold={80}
        />
        <MetricChart
          title="Memory Usage"
          description="System memory percentage over time"
          icon={MemoryStick}
          data={chartData}
          dataKey="mem"
          color="var(--color-secondary-500, #d98a6a)"
          unit="%"
          threshold={85}
        />
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 mb-6">
        <MetricChart
          title="Goroutines"
          description="Active goroutine count over time"
          icon={Zap}
          data={chartData}
          dataKey="goroutines"
          color="#10b981"
          unit=""
        />
        <MetricChart
          title="Heap Memory"
          description="Go heap allocation in MB over time"
          icon={Server}
          data={chartData}
          dataKey="heap"
          color="#8b5cf6"
          unit=" MB"
        />
      </div>

      {/* ── Spike log ────────────────────────────────────────────── */}
      {spikes.length > 0 && (
        <Card className="mb-6">
          <SectionHeader
            title="Spike Events"
            description="Recent CPU (>80%) or Memory (>85%) spikes detected"
            icon={AlertTriangle}
            iconTone="danger"
          />
          <div className="px-6 pb-5">
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-[var(--border)]">
                    <th className="pb-2 pr-4 text-left text-xs font-medium uppercase tracking-wider text-[var(--text-muted)]">Time</th>
                    <th className="pb-2 pr-4 text-left text-xs font-medium uppercase tracking-wider text-[var(--text-muted)]">Type</th>
                    <th className="pb-2 pr-4 text-right text-xs font-medium uppercase tracking-wider text-[var(--text-muted)]">CPU %</th>
                    <th className="pb-2 pr-4 text-right text-xs font-medium uppercase tracking-wider text-[var(--text-muted)]">Mem %</th>
                    <th className="pb-2 text-right text-xs font-medium uppercase tracking-wider text-[var(--text-muted)]">Goroutines</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[var(--border)]">
                  {spikes.map((sp, i) => (
                    <tr key={i} className="group">
                      <td className="py-2 pr-4 font-mono text-xs text-[var(--text-muted)]">{tsLabel(sp.ts)}</td>
                      <td className="py-2 pr-4">
                        {sp.cpu_spike && <Badge tone="danger">CPU spike</Badge>}
                        {sp.mem_spike && <Badge tone="danger">Memory spike</Badge>}
                      </td>
                      <td className="py-2 pr-4 text-right tabular-nums">
                        <span className={sp.cpu_spike ? "text-[color:var(--color-danger)] font-medium" : ""}>
                          {sp.cpu_pct.toFixed(1)}%
                        </span>
                      </td>
                      <td className="py-2 pr-4 text-right tabular-nums">
                        <span className={sp.mem_spike ? "text-[color:var(--color-danger)] font-medium" : ""}>
                          {sp.mem_pct.toFixed(1)}%
                        </span>
                      </td>
                      <td className="py-2 text-right tabular-nums">{sp.goroutines}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </Card>
      )}

      {/* ── Runtime details ──────────────────────────────────────── */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 mb-6">
        <Card>
          <SectionHeader
            title="Go Runtime"
            description="Memory and GC statistics"
            icon={Server}
            iconTone="accent"
          />
          <div className="px-6 pb-5">
            <div className="grid grid-cols-2 gap-x-8 gap-y-3">
              <DetailRow label="Heap Alloc" value={`${s.heap_alloc_mb.toFixed(1)} MB`} />
              <DetailRow label="Heap Sys" value={`${s.heap_sys_mb.toFixed(1)} MB`} />
              <DetailRow label="Heap In-Use" value={`${s.heap_inuse_mb.toFixed(1)} MB`} />
              <DetailRow label="Heap Idle" value={`${s.heap_idle_mb.toFixed(1)} MB`} />
              <DetailRow label="GC Cycles" value={s.gc_cycles.toLocaleString()} />
              <DetailRow label="GC Pause (total)" value={`${s.gc_pause_total_ms.toFixed(1)} ms`} />
              <DetailRow label="GC Pause (last)" value={`${s.gc_pause_last_ms.toFixed(2)} ms`} />
              <DetailRow label="Network Conns" value={s.open_fds.toLocaleString()} />
            </div>
          </div>
        </Card>

        <Card>
          <SectionHeader
            title="Host Info"
            description="System and process details"
            icon={Globe}
            iconTone="secondary"
          />
          <div className="px-6 pb-5">
            <div className="grid grid-cols-2 gap-x-8 gap-y-3">
              <DetailRow label="Hostname" value={s.host || "—"} />
              <DetailRow label="OS" value={s.os || "—"} />
              <DetailRow label="Architecture" value={s.arch || "—"} />
              <DetailRow label="PID" value={String(s.pid)} />
              <DetailRow label="Uptime" value={formatUptime(s.uptime_s)} />
              <DetailRow label="Memory Available" value={`${s.mem_available_mb} MB`} />
              <DetailRow label="Disk Free" value={`${s.disk_free_gb.toFixed(1)} GB`} />
            </div>
          </div>
        </Card>
      </div>

      {/* ── Per-core CPU ─────────────────────────────────────────── */}
      {s.cpu_per_core.length > 0 && (
        <Card className="mb-6">
          <SectionHeader
            title="CPU Per Core"
            description={`Utilization across ${s.cpu_per_core.length} cores`}
            icon={Cpu}
            iconTone="accent"
          />
          <div className="px-6 pb-5">
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
              {s.cpu_per_core.map((pct, i) => (
                <div
                  key={i}
                  className="flex flex-col items-center rounded-xl border border-[var(--border)] bg-[var(--bg)] p-3"
                >
                  <span className="text-[10px] font-medium uppercase tracking-wider text-[var(--text-muted)] mb-1.5">
                    Core {i}
                  </span>
                  <span className={`text-lg font-semibold tabular-nums ${
                    pct >= 85 ? "text-[color:var(--color-danger)]" :
                    pct >= 60 ? "text-[color:var(--color-warning)]" :
                    "text-[var(--text)]"
                  }`}>
                    {pct.toFixed(0)}%
                  </span>
                  {/* mini bar */}
                  <div className="mt-1.5 h-1.5 w-full rounded-full bg-ink-200 dark:bg-ink-700 overflow-hidden">
                    <div
                      className={`h-full rounded-full transition-all duration-500 ${
                        pct >= 85 ? "bg-[color:var(--color-danger)]" :
                        pct >= 60 ? "bg-[color:var(--color-warning)]" :
                        "bg-accent-500"
                      }`}
                      style={{ width: `${Math.min(pct, 100)}%` }}
                    />
                  </div>
                </div>
              ))}
            </div>
          </div>
        </Card>
      )}
    </>
  );
}

// ---- sub-components ---------------------------------------------------------

function MetricChart({
  title,
  description,
  icon: Icon,
  data,
  dataKey,
  color,
  unit,
  threshold,
}: {
  title: string;
  description: string;
  icon: typeof Activity;
  data: Record<string, string | number>[];
  dataKey: string;
  color: string;
  unit: string;
  threshold?: number;
}) {
  return (
    <Card>
      <SectionHeader title={title} description={description} icon={Icon} iconTone="accent" />
      <div className="px-4 pb-4" style={{ height: 220 }}>
        {data.length < 2 ? (
          <div className="flex h-full items-center justify-center text-sm text-[var(--text-muted)]">
            Collecting data…
          </div>
        ) : (
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={data} margin={{ top: 4, right: 8, left: -12, bottom: 0 }}>
              <defs>
                <linearGradient id={`grad-${dataKey}`} x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor={color} stopOpacity={0.3} />
                  <stop offset="100%" stopColor={color} stopOpacity={0.02} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
              <XAxis
                dataKey="time"
                tick={{ fontSize: 10, fill: "var(--text-muted)" }}
                tickLine={false}
                axisLine={false}
              />
              <YAxis
                tick={{ fontSize: 10, fill: "var(--text-muted)" }}
                tickLine={false}
                axisLine={false}
                domain={unit === "%" ? [0, 100] : ["auto", "auto"]}
              />
              <Tooltip
                contentStyle={{
                  backgroundColor: "var(--bg-elevated)",
                  border: "1px solid var(--border)",
                  borderRadius: 12,
                  fontSize: 12,
                  boxShadow: "var(--shadow-pop)",
                }}
                labelStyle={{ fontSize: 11, color: "var(--text-muted)" }}
                formatter={(val: number) => [`${val}${unit}`, title]}
              />
              {threshold != null && (
                <Area
                  type="monotone"
                  dataKey={() => threshold}
                  stroke="#ef4444"
                  strokeDasharray="4 4"
                  strokeWidth={1}
                  fill="none"
                  dot={false}
                  activeDot={false}
                  isAnimationActive={false}
                />
              )}
              <Area
                type="monotone"
                dataKey={dataKey}
                stroke={color}
                strokeWidth={2}
                fill={`url(#grad-${dataKey})`}
                dot={false}
                activeDot={{ r: 4, stroke: color, strokeWidth: 2, fill: "var(--bg-elevated)" }}
              />
            </AreaChart>
          </ResponsiveContainer>
        )}
      </div>
    </Card>
  );
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline justify-between gap-2">
      <span className="text-xs text-[var(--text-muted)]">{label}</span>
      <span className="text-sm font-medium tabular-nums">{value}</span>
    </div>
  );
}