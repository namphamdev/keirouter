import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../lib/api";
import { PageHeader } from "../components/Layout";
import { Card, Select, Spinner } from "../components/ui";

const periods = [
  { value: "today", label: "Today" },
  { value: "week", label: "Last 7 days" },
  { value: "month", label: "Last 30 days" },
];

export function OverviewPage() {
  const [period, setPeriod] = useState("month");
  const usage = useQuery({ queryKey: ["usage", period], queryFn: () => api.usage(period) });

  return (
    <>
      <PageHeader title="Overview" description="Usage and spending across all providers." />

      <div className="mb-4 flex justify-end">
        <div className="w-40">
          <Select value={period} onChange={(e) => setPeriod(e.target.value)}>
            {periods.map((p) => (
              <option key={p.value} value={p.value}>
                {p.label}
              </option>
            ))}
          </Select>
        </div>
      </div>

      {usage.isLoading ? (
        <Spinner />
      ) : usage.isError ? (
        <Card className="px-5 py-8 text-center text-sm text-[color:var(--color-danger)]">
          Failed to load usage. Is the backend running?
        </Card>
      ) : (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <Stat label="Requests" value={usage.data!.total_requests.toLocaleString()} />
          <Stat label="Est. cost" value={`$${usage.data!.cost_usd.toFixed(2)}`} />
          <Stat
            label="Tokens in / out"
            value={`${compact(usage.data!.prompt_tokens)} / ${compact(usage.data!.completion_tokens)}`}
          />
          <Stat label="Cache hits" value={usage.data!.cache_hits.toLocaleString()} />
        </div>
      )}

      <Card className="mt-6 p-5">
        <h2 className="text-sm font-semibold tracking-tight">Getting started</h2>
        <ol className="mt-3 space-y-2 text-sm text-[var(--text-muted)]">
          <li>1. Add a provider account under Accounts.</li>
          <li>2. Create a routing chain to define fallback order.</li>
          <li>3. Create an API key and point your tool at it.</li>
        </ol>
        <pre className="mt-4 overflow-x-auto rounded-md bg-ink-100 p-3 font-mono text-xs dark:bg-ink-800">
{`Base URL: http://localhost:20180/v1
Model:    openai/gpt-4o   or   chain:my-chain`}
        </pre>
      </Card>
    </>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <Card className="p-4">
      <p className="text-xs text-[var(--text-muted)]">{label}</p>
      <p className="mt-1 text-xl font-semibold tracking-tight">{value}</p>
    </Card>
  );
}

function compact(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}