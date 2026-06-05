import { useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { fetchKeyUsage, fetchKeyUsageById, APIError } from "../lib/api";
import { KeyRound, AlertTriangle, CheckCircle2, Activity, Hash, DollarSign, LogOut } from "lucide-react";
import { Card, Button, Input, Spinner, ErrorCard, StatCard, Badge } from "../components/ui";

export function KeyPortalPage() {
  const [params, setParams] = useSearchParams();
  const activeId = params.get("id") || "";
  const activeKey = params.get("key") || "";
  const [apiKeyInput, setApiKeyInput] = useState(activeKey || activeId);

  const authValue = activeId || activeKey;
  const isIdMode = !!activeId;

  const handleLogin = (e: React.FormEvent) => {
    e.preventDefault();
    const val = apiKeyInput.trim();
    if (val) {
      if (val.startsWith("sk-")) {
        setParams({ key: val });
      } else {
        setParams({ id: val });
      }
    }
  };

  const handleLogout = () => {
    setParams({});
    setApiKeyInput("");
  };

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["key-usage", authValue, isIdMode],
    queryFn: () => isIdMode ? fetchKeyUsageById(authValue) : fetchKeyUsage(authValue),
    enabled: !!authValue,
    retry: false,
    refetchInterval: 30000,
  });

  if (!authValue) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-[var(--bg)] p-4">
        <Card className="w-full max-w-md p-8">
          <div className="mb-8 flex flex-col items-center text-center">
            <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-accent-100 text-accent-700 dark:bg-accent-800/40 dark:text-accent-200 mb-5">
              <KeyRound size={24} />
            </div>
            <h1 className="text-xl font-display text-[var(--text)]">Telemetry Portal</h1>
            <p className="mt-2 text-sm text-[var(--text-muted)]">
              Enter your API Key or secure Portal ID to view live usage and budget constraints.
            </p>
          </div>

          <form onSubmit={handleLogin} className="space-y-5">
            <div className="space-y-1.5">
              <label className="text-xs font-medium text-[var(--text-muted)]">Key or Portal ID</label>
              <Input
                type="password"
                value={apiKeyInput}
                onChange={(e) => setApiKeyInput(e.target.value)}
                placeholder="sk-... or key_..."
                autoFocus
              />
            </div>
            <Button 
              type="submit" 
              className="w-full"
              disabled={!apiKeyInput.trim()}
            >
              Authenticate
            </Button>
          </form>
        </Card>
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-[var(--bg)]">
        <Spinner />
      </div>
    );
  }

  if (isError) {
    let msg = "Authentication failed or server error.";
    if (error instanceof APIError) {
      msg = error.message;
    }
    return (
      <div className="flex min-h-screen items-center justify-center bg-[var(--bg)] p-4">
        <div className="w-full max-w-md space-y-4 text-center">
          <ErrorCard message={msg} />
          <Button variant="ghost" onClick={handleLogout}>
            Return to Login
          </Button>
        </div>
      </div>
    );
  }

  const d = data!;

  return (
    <div className="min-h-screen bg-[var(--bg)] p-4 md:p-8">
      <div className="mx-auto max-w-5xl space-y-8">
        
        {/* Header Section */}
        <header className="flex flex-col gap-4 md:flex-row md:items-end justify-between border-b border-[var(--border)] pb-6">
          <div>
            <div className="flex items-center gap-3 mb-2">
              <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-accent-100 text-accent-700 dark:bg-accent-800/40 dark:text-accent-200">
                <Activity size={20} />
              </div>
              <div>
                <h1 className="text-2xl font-display text-[var(--text)]">Key Telemetry</h1>
                <p className="text-sm text-[var(--text-muted)]">
                  Monitoring usage for <span className="font-medium text-[var(--text)]">{d.key_name}</span>
                </p>
              </div>
            </div>
            <div className="flex items-center gap-2 text-xs font-mono text-[var(--text-muted)] mt-4">
              <span>ID: {d.key_id}</span>
            </div>
          </div>
          <Button variant="ghost" onClick={handleLogout} className="self-start md:self-auto">
            <LogOut size={16} /> Disconnect
          </Button>
        </header>

        {/* ── Budget Overview ──────────────────────── */}
        {d.budgets && d.budgets.length > 0 ? (
          <section className="space-y-4">
            <h2 className="text-sm font-semibold tracking-tight text-[var(--text)]">Budget Overview</h2>
            <div className="grid gap-4 md:grid-cols-2">
              {d.budgets.map((b, i) => (
                <Card key={i} className="p-6 relative overflow-hidden">
                  <div className={`absolute top-0 left-0 w-1 h-full ${b.alert ? 'bg-[color:var(--color-danger)]' : 'bg-accent-500'}`} />
                  
                  <div className="flex justify-between items-start mb-6">
                    <div>
                      <h3 className="font-semibold text-[var(--text)]">
                        {b.period === 'total' ? 'All-Time' : b.period.charAt(0).toUpperCase() + b.period.slice(1)} Budget
                      </h3>
                      <p className="text-xs text-[var(--text-muted)] mt-0.5 capitalize">{b.period} cycle</p>
                    </div>
                    {b.alert && (
                      <Badge tone="danger">
                        <span className="flex items-center gap-1">
                          <AlertTriangle size={12} /> Exceeded
                        </span>
                      </Badge>
                    )}
                  </div>

                  <div className="space-y-6">
                    {/* Token Gauge */}
                    {b.limit_tokens > 0 && (
                      <div>
                        <div className="flex items-end justify-between mb-1">
                          <span className="text-sm font-medium text-[var(--text)]">Tokens</span>
                          <span className="text-sm font-medium text-[var(--text)]">
                            {formatTokens(b.tokens_used)} <span className="text-[var(--text-muted)] font-normal">/ {formatTokens(b.limit_tokens)}</span>
                          </span>
                        </div>
                        <ProgressBar pct={b.tokens_pct_used} alert={b.alert} />
                        <div className="flex justify-end mt-1.5 text-xs text-[var(--text-muted)]">
                          {formatTokens(b.tokens_remaining)} remaining
                        </div>
                      </div>
                    )}

                    {/* USD Gauge */}
                    {b.limit_usd > 0 && (
                      <div>
                        <div className="flex items-end justify-between mb-1">
                          <span className="text-sm font-medium text-[var(--text)]">Spend (USD)</span>
                          <span className="text-sm font-medium text-[var(--text)]">
                            ${b.spent_usd.toFixed(4)} <span className="text-[var(--text-muted)] font-normal">/ ${b.limit_usd.toFixed(2)}</span>
                          </span>
                        </div>
                        <ProgressBar pct={b.usd_pct_used} alert={b.alert} />
                        <div className="flex justify-end mt-1.5 text-xs text-[var(--text-muted)]">
                          ${b.usd_remaining.toFixed(4)} remaining
                        </div>
                      </div>
                    )}
                  </div>
                </Card>
              ))}
            </div>
          </section>
        ) : (
          <section className="space-y-4">
            <h2 className="text-sm font-semibold tracking-tight text-[var(--text)]">Budget Overview</h2>
            <Card className="flex flex-col items-center justify-center p-12 text-center">
              <p className="text-sm font-medium text-[var(--text)]">No Budgets Configured</p>
              <p className="text-sm text-[var(--text-muted)] mt-1">This key has unlimited usage.</p>
            </Card>
          </section>
        )}

        {/* ── Current Period Usage Stats ─────────────────────────────── */}
        <section className="space-y-4">
          <h2 className="text-sm font-semibold tracking-tight text-[var(--text)]">Current Period Usage</h2>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <StatCard 
              icon={Activity} 
              label="Total Requests" 
              value={d.current_period.total_requests.toLocaleString()} 
            />
            <StatCard 
              icon={Hash} 
              label="Prompt Tokens" 
              value={formatTokens(d.current_period.prompt_tokens)} 
            />
            <StatCard 
              icon={Hash} 
              label="Completion Tokens" 
              value={formatTokens(d.current_period.completion_tokens)} 
            />
            <StatCard 
              icon={DollarSign} 
              iconTone={d.current_period.cost_usd > 0 ? "accent" : undefined}
              label="Accrued Cost" 
              value={`$${d.current_period.cost_usd.toFixed(4)}`} 
            />
          </div>
        </section>

        {/* ── Allowed Models Panel ──────────────────────────────────── */}
        {d.allowed_models && d.allowed_models.length > 0 && (
          <section className="space-y-4">
            <h2 className="text-sm font-semibold tracking-tight text-[var(--text)]">Authorized Models</h2>
            <Card className="p-4">
              <div className="flex flex-wrap gap-2">
                {d.allowed_models.map(m => (
                  <Badge key={m} tone="neutral">
                    <span className="flex items-center gap-1.5 font-mono">
                      <CheckCircle2 size={12} className="text-accent-500" />
                      {m}
                    </span>
                  </Badge>
                ))}
              </div>
            </Card>
          </section>
        )}

      </div>
    </div>
  );
}

function formatTokens(n: number): string {
  if (n >= 1_000_000_000) return `${(n / 1_000_000_000).toFixed(1)}B`;
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return n.toLocaleString();
}

function ProgressBar({ pct, alert }: { pct: number, alert: boolean }) {
  const safePct = Math.min(Math.max(pct, 0), 100);
  const colorClass = alert 
    ? "bg-[color:var(--color-danger)]" 
    : safePct > 80 ? "bg-[color:var(--color-warning)]" : "bg-accent-500";
    
  return (
    <div className="h-2.5 w-full overflow-hidden rounded-full bg-[var(--bg-subtle)]">
      <div 
        className={`h-full rounded-full transition-all duration-1000 ease-out ${colorClass}`}
        style={{ width: `${safePct}%` }}
      />
    </div>
  );
}
