import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Boxes, Search, X } from "lucide-react";
import { api, type Provider, type Account } from "../lib/api";
import { PageHeader } from "../components/Layout";
import { Card, CardHeader, Badge, Spinner, EmptyState, StatusDot } from "../components/ui";

// kindFilters are the service-kind tabs shown above the provider grid.
const kindFilters = [
  { id: "all", label: "All" },
  { id: "llm", label: "Chat" },
  { id: "embedding", label: "Embeddings" },
  { id: "image", label: "Image" },
  { id: "stt", label: "Speech-to-Text" },
  { id: "tts", label: "Text-to-Speech" },
  { id: "search", label: "Web Search" },
  { id: "fetch", label: "Web Fetch" },
];

export function ProvidersPage() {
  const providers = useQuery({ queryKey: ["providers"], queryFn: () => api.providers() });
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: () => api.listAccounts() });
  const [filter, setFilter] = useState("all");
  const [searchQuery, setSearchQuery] = useState("");

  // Count accounts per provider id so we can split connected vs available.
  const accountsByProvider = useMemo(() => {
    const map = new Map<string, Account[]>();
    for (const a of accounts.data?.accounts ?? []) {
      const list = map.get(a.provider) ?? [];
      list.push(a);
      map.set(a.provider, list);
    }
    return map;
  }, [accounts.data]);

  const visible = useMemo(() => {
    const all = providers.data?.providers ?? [];
    return all
      .filter((p) => !p.hidden)
      .filter((p) => filter === "all" || p.service_kinds.includes(filter))
      .filter((p) => {
        if (!searchQuery.trim()) return true;
        const q = searchQuery.toLowerCase();
        return (
          p.display_name.toLowerCase().includes(q) ||
          p.id.toLowerCase().includes(q) ||
          p.alias.toLowerCase().includes(q)
        );
      });
  }, [providers.data, filter, searchQuery]);

  const connected = visible.filter((p) => accountsByProvider.has(p.id));
  const available = visible.filter((p) => !accountsByProvider.has(p.id));

  return (
    <>
      <PageHeader
        title="Providers"
        icon={Boxes}
        description="Connect and manage AI providers to power your routing."
      />

      <div className="mb-5 flex flex-col gap-3 sm:flex-row sm:items-center">
        <div className="flex flex-wrap gap-1.5">
          {kindFilters.map((k) => (
            <button
              key={k.id}
              onClick={() => setFilter(k.id)}
              className={`rounded-xl px-3.5 py-2 text-xs font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/60 ${
                filter === k.id
                  ? "bg-accent-600 text-white shadow-sm"
                  : "border border-[var(--border)] bg-[var(--bg-elevated)] text-[var(--text-muted)] hover:bg-ink-100 dark:hover:bg-ink-800"
              }`}
            >
              {k.label}
            </button>
          ))}
        </div>
        <div className="relative max-w-sm flex-1">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--text-muted)]" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Search providers…"
            className="w-full rounded-xl border border-[var(--border)] bg-[var(--bg-elevated)] py-2 pl-9 pr-9 text-sm placeholder:text-[var(--text-muted)] focus:border-accent-400 focus:outline-none focus:ring-2 focus:ring-accent-400/40"
          />
          {searchQuery && (
            <button
              onClick={() => setSearchQuery("")}
              className="absolute right-2 top-1/2 -translate-y-1/2 rounded p-0.5 text-[var(--text-muted)] hover:text-[var(--text)]"
              aria-label="Clear search"
            >
              <X className="h-4 w-4" />
            </button>
          )}
        </div>
      </div>

      {providers.isLoading ? (
        <Spinner />
      ) : (
        <div className="space-y-6">
          <Card>
            <CardHeader
              title="Connected providers"
              description="These providers have accounts and are ready to use."
              action={<Badge tone="accent">{connected.length}</Badge>}
            />
            {!connected.length ? (
              <EmptyState
                title={searchQuery ? `No connected providers match "${searchQuery}"` : "No connected providers yet"}
                hint={searchQuery ? "Try a different search term." : "Pick a provider below to add your first account."}
              />
            ) : (
              <div className="grid grid-cols-2 gap-px overflow-hidden rounded-b-2xl bg-[var(--border)] sm:grid-cols-3 lg:grid-cols-4">
                {connected.map((p) => (
                  <ProviderCard
                    key={p.id}
                    provider={p}
                    accountCount={accountsByProvider.get(p.id)?.length ?? 0}
                  />
                ))}
              </div>
            )}
          </Card>

          <Card>
            <CardHeader
              title="Available providers"
              description="Add new providers to expand your routing options."
              action={<Badge tone="neutral">{available.length}</Badge>}
            />
            {!available.length ? (
              <EmptyState
                title={searchQuery ? `No providers match "${searchQuery}"` : "No providers for this capability"}
              />
            ) : (
              <div className="grid grid-cols-2 gap-px overflow-hidden rounded-b-2xl bg-[var(--border)] sm:grid-cols-3 lg:grid-cols-4">
                {available.map((p) => (
                  <ProviderCard key={p.id} provider={p} accountCount={0} />
                ))}
              </div>
            )}
          </Card>
        </div>
      )}
    </>
  );
}

function ProviderCard({ provider: p, accountCount }: { provider: Provider; accountCount: number }) {
  const navigate = useNavigate();
  const connected = accountCount > 0;

  return (
    <button
      onClick={() => navigate(`/providers/${p.id}`)}
      className="flex h-full flex-col items-start gap-3 bg-[var(--bg-elevated)] p-5 text-left transition-colors hover:bg-ink-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-accent-400/60 dark:hover:bg-ink-800/40"
    >
      <div className="flex w-full items-start justify-between gap-2">
        <ProviderIcon provider={p} />
        {connected ? (
          <span className="inline-flex items-center gap-1.5 rounded-md bg-accent-100 px-2 py-0.5 text-xs font-medium text-accent-700 dark:bg-accent-800/40 dark:text-accent-200">
            <StatusDot tone="success" />
            Connected
          </span>
        ) : p.deprecated ? (
          <Badge tone="danger">risk</Badge>
        ) : !p.drivable ? (
          <Badge tone="neutral">soon</Badge>
        ) : null}
      </div>

      <div className="min-w-0">
        <p className="truncate text-sm font-semibold">{p.display_name}</p>
        <p className="mt-0.5 truncate font-mono text-xs text-[var(--text-muted)]">{p.id}</p>
      </div>

      <p className="mt-auto text-xs text-[var(--text-muted)]">
        {connected
          ? `${accountCount} ${accountCount === 1 ? "account" : "accounts"}`
          : "Connect"}
      </p>
    </button>
  );
}

// ProviderIcon renders the provider PNG with a colored fallback initial.
function ProviderIcon({ provider: p }: { provider: Provider }) {
  const [errored, setErrored] = useState(false);
  if (errored || !p.icon) {
    return (
      <div
        className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl text-sm font-bold text-white"
        style={{ backgroundColor: p.color || "var(--text-muted)" }}
      >
        {p.display_name.slice(0, 1).toUpperCase()}
      </div>
    );
  }
  return (
    <img
      src={p.icon}
      alt={p.display_name}
      onError={() => setErrored(true)}
      className="h-10 w-10 shrink-0 rounded-xl object-contain"
    />
  );
}