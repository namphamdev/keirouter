import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Wallet, Plus, Trash2, Pencil, AlertTriangle, ShieldCheck, KeyRound, Building2 } from "lucide-react";
import { api, type BudgetStatus, type APIKey } from "../lib/api";
import { PageHeader } from "../components/Layout";
import { useToast } from "../components/Toast";
import {
  Card,
  SectionHeader,
  Button,
  Input,
  Select,
  Field,
  Badge,
  Spinner,
  EmptyState,
  ErrorBanner,
  Toggle,
  Modal,
} from "../components/ui";

const periods = [
  { value: "daily", label: "Daily" },
  { value: "weekly", label: "Weekly" },
  { value: "monthly", label: "Monthly" },
  { value: "total", label: "All time" },
];

function microsToUSD(micros: number): string {
  return `$${(micros / 1_000_000).toFixed(2)}`;
}

function formatTokens(n: number): string {
  if (n >= 1_000_000_000) return `${(n / 1_000_000_000).toFixed(1)}B`;
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return n.toString();
}

function progressColor(pct: number, alertPct: number): string {
  if (pct >= 100) return "bg-red-500";
  if (pct >= alertPct) return "bg-amber-500";
  if (pct >= alertPct * 0.75) return "bg-amber-400";
  return "bg-emerald-500";
}

// Format number with thousand separators: 1000000 → "1.000.000"
function formatTokenLimit(value: string): string {
  if (!value) return "";
  const n = parseInt(value.replace(/\D/g, ""), 10);
  if (isNaN(n)) return "";
  return n.toLocaleString("id-ID");
}

/* ── Formatted Token Input ─────────────────────────────────────────── */

function FormattedTokenInput({
  value,
  onChange,
  placeholder,
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
}) {
  const [focused, setFocused] = useState(false);
  const formatted = formatTokenLimit(value);

  return (
    <input
      type="text"
      inputMode="numeric"
      value={focused ? value : formatted}
      onFocus={() => setFocused(true)}
      onBlur={() => setFocused(false)}
      onChange={(e) => {
        const raw = e.target.value.replace(/[^\d]/g, "");
        onChange(raw);
      }}
      placeholder={placeholder ? formatTokenLimit(placeholder) : undefined}
      className="w-full rounded-xl border border-[var(--border)] bg-[var(--bg-elevated)] px-3 py-2 text-sm placeholder:text-[var(--text-muted)] focus:border-accent-400 focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/40"
    />
  );
}

export function BudgetsPage() {
  const qc = useQueryClient();
  const toast = useToast();

  const status = useQuery({
    queryKey: ["budget-status"],
    queryFn: () => api.budgetStatus(),
    refetchInterval: 30_000,
  });

  const keys = useQuery({
    queryKey: ["keys"],
    queryFn: () => api.listKeys(),
  });

  const [showCreate, setShowCreate] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);

  const remove = useMutation({
    mutationFn: (id: string) => api.deleteBudget(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["budget-status"] });
      qc.invalidateQueries({ queryKey: ["budgets"] });
      toast.success("Budget removed", "The spend limit has been deleted. Requests matching this scope are no longer capped.");
    },
    onError: (e: Error) => toast.error("Budget removal failed", e.message),
  });

  const budgets = status.data?.budgets ?? [];
  const editingBudget = budgets.find((b) => b.id === editingId);

  return (
    <>
      <PageHeader
        title="Budgets"
        icon={Wallet}
        description="Hard spend caps per key or tenant. Requests are auto-blocked when a budget is exhausted."
        action={
          <Button onClick={() => setShowCreate(true)}>
            <Plus className="h-4 w-4" />
            New budget
          </Button>
        }
      />

      {/* ── Alerts ─────────────────────────────────────────────── */}
      {budgets.some((b) => b.pct_used >= 100 && b.hard_cutoff) && (
        <Card className="mb-6 border-red-300 bg-red-50 dark:border-red-800 dark:bg-red-950/30">
          <div className="flex items-center gap-3 px-6 py-4">
            <AlertTriangle className="h-5 w-5 shrink-0 text-red-600 dark:text-red-400" />
            <div>
              <p className="text-sm font-medium text-red-800 dark:text-red-200">
                Budget exhausted — requests are being blocked
              </p>
              <p className="text-xs text-red-600 dark:text-red-400">
                {budgets
                  .filter((b) => (b.limit_micros > 0 && b.pct_used >= 100) || (b.limit_tokens > 0 && b.tokens_pct_used >= 100))
                  .filter((b) => b.hard_cutoff)
                  .map((b) => `${b.scope_name} (${b.period})`)
                  .join(", ")}
              </p>
            </div>
          </div>
        </Card>
      )}

      {budgets.some((b) => {
        const usdAlert = b.limit_micros > 0 && b.pct_used >= b.alert_pct && b.pct_used < 100;
        const tokAlert = b.limit_tokens > 0 && b.tokens_pct_used >= b.alert_pct && b.tokens_pct_used < 100;
        return usdAlert || tokAlert;
      }) && (
        <Card className="mb-6 border-amber-300 bg-amber-50 dark:border-amber-800 dark:bg-amber-950/30">
          <div className="flex items-center gap-3 px-6 py-4">
            <AlertTriangle className="h-5 w-5 shrink-0 text-amber-600 dark:text-amber-400" />
            <div>
              <p className="text-sm font-medium text-amber-800 dark:text-amber-200">
                Budget alert threshold reached
              </p>
              <p className="text-xs text-amber-600 dark:text-amber-400">
                {budgets
                  .filter((b) => {
                    const usdAlert = b.limit_micros > 0 && b.pct_used >= b.alert_pct && b.pct_used < 100;
                    const tokAlert = b.limit_tokens > 0 && b.tokens_pct_used >= b.alert_pct && b.tokens_pct_used < 100;
                    return usdAlert || tokAlert;
                  })
                  .map((b) => {
                    const parts = [];
                    if (b.limit_micros > 0) parts.push(`$${b.pct_used.toFixed(0)}%`);
                    if (b.limit_tokens > 0) parts.push(`tok ${b.tokens_pct_used.toFixed(0)}%`);
                    return `${b.scope_name}: ${parts.join(" / ")}`;
                  })
                  .join(", ")}
              </p>
            </div>
          </div>
        </Card>
      )}

      {/* ── Create Modal ────────────────────────────────────────── */}
      <Modal
        open={showCreate}
        onClose={() => setShowCreate(false)}
        title="Create budget"
        subtitle="Set a spending cap for a scope and period."
      >
        <CreateBudgetForm
          keys={keys.data?.keys ?? []}
          onClose={() => setShowCreate(false)}
        />
      </Modal>

      {/* ── Edit Modal ──────────────────────────────────────────── */}
      <Modal
        open={!!editingId}
        onClose={() => setEditingId(null)}
        title="Edit budget"
        subtitle={editingBudget ? `Editing ${editingBudget.scope_name} ${editingBudget.period} budget` : undefined}
      >
        {editingBudget && (
          <EditBudgetForm
            budget={editingBudget}
            onClose={() => setEditingId(null)}
          />
        )}
      </Modal>

      {/* ── Budget list ────────────────────────────────────────── */}
      <Card>
        <SectionHeader
          title="Active budgets"
          description="Spend limits with live usage tracking."
          icon={ShieldCheck}
        />
        {status.isLoading ? (
          <div className="px-6 pb-6">
            <Spinner />
          </div>
        ) : budgets.length === 0 ? (
          <div className="px-6 pb-6">
            <EmptyState title="No budgets set" hint="Spending is unlimited until you add a budget." />
          </div>
        ) : (
          <div className="divide-y divide-[var(--border)]">
            {budgets.map((b) => (
              <BudgetRow
                key={b.id}
                budget={b}
                onEdit={() => setEditingId(b.id)}
                onDelete={() => {
                  if (confirm(`Remove this ${microsToUSD(b.limit_micros)} ${b.period} budget?`)) {
                    remove.mutate(b.id);
                  }
                }}
              />
            ))}
          </div>
        )}
      </Card>
    </>
  );
}

/* ── Budget row ──────────────────────────────────────────────────── */

function BudgetRow({
  budget: b,
  onEdit,
  onDelete,
}: {
  budget: BudgetStatus;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const remaining = Math.max(0, b.limit_micros - b.spent_micros);
  const overLimit = b.pct_used >= 100;

  return (
    <div className="px-6 py-5">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0 flex-1">
          {/* Header badges */}
          <div className="flex flex-wrap items-center gap-2">
              <span className="text-sm font-medium">
                {b.limit_micros > 0 ? microsToUSD(b.limit_micros) : "—"}
                {b.limit_tokens > 0 && (
                  <span className="ml-2 text-[var(--text-muted)]">{formatTokens(b.limit_tokens)} tok</span>
                )}
              </span>
            <Badge>{b.period}</Badge>
            <Badge tone={b.scope_kind === "api_key" ? "accent" : "neutral"}>
              {b.scope_kind === "api_key" ? (
                <span className="flex items-center gap-1">
                  <KeyRound className="h-3 w-3" />
                  {b.scope_name}
                </span>
              ) : (
                <span className="flex items-center gap-1">
                  <Building2 className="h-3 w-3" />
                  {b.scope_name}
                </span>
              )}
            </Badge>
            {b.hard_cutoff ? (
              <Badge tone="danger">hard cutoff</Badge>
            ) : (
              <Badge tone="neutral">advisory</Badge>
            )}
            {overLimit && b.hard_cutoff && (
              <Badge tone="danger">BLOCKING</Badge>
            )}
          </div>

          {/* USD Progress bar */}
          {b.limit_micros > 0 && (
            <div className="mt-3">
              <div className="flex items-center justify-between text-xs">
                <span className="text-[var(--text-muted)]">
                  {microsToUSD(b.spent_micros)} spent
                </span>
                <span className={overLimit ? "font-medium text-red-600 dark:text-red-400" : "text-[var(--text-muted)]"}>
                  {b.pct_used.toFixed(1)}% used
                </span>
              </div>
              <div className="mt-1.5 relative h-2.5 overflow-hidden rounded-full bg-[var(--bg-subtle)]">
                <div
                  className="absolute top-0 bottom-0 w-px bg-amber-400/60 z-10"
                  style={{ left: `${Math.min(b.alert_pct, 100)}%` }}
                  title={`Alert at ${b.alert_pct}%`}
                />
                <div
                  className={`h-full rounded-full transition-all duration-700 ${progressColor(b.pct_used, b.alert_pct)}`}
                  style={{ width: `${Math.min(b.pct_used, 100)}%` }}
                />
              </div>
              <div className="mt-1.5 flex items-center justify-between text-xs text-[var(--text-muted)]">
                <span>{microsToUSD(remaining)} remaining</span>
                <span>alert at {b.alert_pct}%</span>
              </div>
            </div>
          )}
          {/* Token Progress bar */}
          {b.limit_tokens > 0 && (
            <div className="mt-3">
              <div className="flex items-center justify-between text-xs">
                <span className="text-[var(--text-muted)]">
                  {formatTokens(b.spent_tokens)} tokens used
                </span>
                <span className={b.tokens_pct_used >= 100 ? "font-medium text-red-600 dark:text-red-400" : "text-[var(--text-muted)]"}>
                  {b.tokens_pct_used.toFixed(1)}% used
                </span>
              </div>
              <div className="mt-1.5 relative h-2.5 overflow-hidden rounded-full bg-[var(--bg-subtle)]">
                <div
                  className="absolute top-0 bottom-0 w-px bg-amber-400/60 z-10"
                  style={{ left: `${Math.min(b.alert_pct, 100)}%` }}
                  title={`Alert at ${b.alert_pct}%`}
                />
                <div
                  className={`h-full rounded-full transition-all duration-700 ${progressColor(b.tokens_pct_used, b.alert_pct)}`}
                  style={{ width: `${Math.min(b.tokens_pct_used, 100)}%` }}
                />
              </div>
              <div className="mt-1.5 flex items-center justify-between text-xs text-[var(--text-muted)]">
                <span>{formatTokens(Math.max(0, b.limit_tokens - b.spent_tokens))} remaining</span>
                <span>alert at {b.alert_pct}%</span>
              </div>
            </div>
          )}
        </div>

        {/* Actions */}
        <div className="flex shrink-0 items-center gap-1.5">
          <Button variant="ghost" onClick={onEdit} className="px-2" title="Edit budget">
            <Pencil className="h-4 w-4" />
          </Button>
          <Button variant="danger" onClick={onDelete} className="px-2" title="Remove budget">
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  );
}

/* ── Create form ─────────────────────────────────────────────────── */

function CreateBudgetForm({ keys, onClose }: { keys: APIKey[]; onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();

  const [scopeKind, setScopeKind] = useState<string>("tenant");
  const [scopeId, setScopeId] = useState("");
  const [limit, setLimit] = useState("");
  const [limitTokens, setLimitTokens] = useState("");
  const [period, setPeriod] = useState("monthly");
  const [alertPct, setAlertPct] = useState(80);
  const [hardCutoff, setHardCutoff] = useState(true);
  const [error, setError] = useState("");

  const create = useMutation({
    mutationFn: () =>
      api.createBudget({
        scope_kind: scopeKind,
        scope_id: scopeKind === "api_key" && scopeId ? scopeId : undefined,
        limit_usd: parseFloat(limit) || undefined,
        limit_tokens: parseInt(limitTokens) || undefined,
        period,
        alert_pct: alertPct,
        hard_cutoff: hardCutoff,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["budget-status"] });
      qc.invalidateQueries({ queryKey: ["budgets"] });
      const parts = [];
      if (parseFloat(limit) > 0) parts.push(`$${parseFloat(limit).toFixed(2)}`);
      if (parseInt(limitTokens) > 0) parts.push(`${formatTokens(parseInt(limitTokens))} tokens`);
      toast.success(
        "Budget created",
        `${parts.join(" + ")} ${period} limit set for ${scopeKind === "api_key" ? "API key" : "tenant"}.${hardCutoff ? " Requests will be blocked when exhausted." : ""}`,
      );
      onClose();
    },
    onError: (e: Error) => {
      setError(e.message);
      toast.error("Budget creation failed", e.message);
    },
  });

  return (
    <form
      className="space-y-4 px-6 py-5"
      onSubmit={(e) => {
        e.preventDefault();
        if (parseFloat(limit) > 0 || parseInt(limitTokens) > 0) create.mutate();
      }}
    >
      {/* Scope selector */}
      <div className="flex gap-3">
        <div className="w-44">
          <Field label="Scope">
            <Select
              value={scopeKind}
              onChange={(e) => {
                setScopeKind(e.target.value);
                setScopeId("");
              }}
            >
              <option value="tenant">Tenant (global)</option>
              <option value="api_key">API Key</option>
            </Select>
          </Field>
        </div>
        {scopeKind === "api_key" && (
          <div className="flex-1">
            <Field label="API Key">
              <Select value={scopeId} onChange={(e) => setScopeId(e.target.value)}>
                <option value="">Select a key…</option>
                {keys.map((k) => (
                  <option key={k.id} value={k.id}>
                    {k.name} ({k.display})
                  </option>
                ))}
              </Select>
            </Field>
          </div>
        )}
      </div>

      {/* Limit (USD) + Limit (Tokens) + Period */}
      <div className="flex gap-3">
        <div className="flex-1">
          <Field label="Limit (USD)">
            <Input
              type="number"
              min="0"
              step="0.01"
              value={limit}
              onChange={(e) => setLimit(e.target.value)}
              placeholder="50.00"
            />
          </Field>
        </div>
        <div className="flex-1">
          <Field label="Limit (Tokens)">
            <FormattedTokenInput
              value={limitTokens}
              onChange={setLimitTokens}
              placeholder="100000000"
            />
          </Field>
        </div>
        <div className="w-36">
          <Field label="Period">
            <Select value={period} onChange={(e) => setPeriod(e.target.value)}>
              {periods.map((p) => (
                <option key={p.value} value={p.value}>
                  {p.label}
                </option>
              ))}
            </Select>
          </Field>
        </div>
      </div>

      {/* Alert + Cutoff */}
      <div className="flex items-end gap-6">
        <div className="w-40">
          <Field label="Alert threshold (%)">
            <Input
              type="number"
              min="1"
              max="100"
              value={alertPct}
              onChange={(e) => setAlertPct(parseInt(e.target.value) || 80)}
            />
          </Field>
        </div>
        <div className="flex items-center gap-2 pb-0.5">
          <Toggle checked={hardCutoff} onChange={setHardCutoff} />
          <span className="text-sm">Hard cutoff (block when exhausted)</span>
        </div>
      </div>

      {error && <ErrorBanner message={error} />}

      <div className="flex gap-2 pt-2 border-t border-[var(--border)]">
        <div className="flex-1" />
        <Button variant="ghost" type="button" onClick={onClose}>
          Cancel
        </Button>
        <Button type="submit" disabled={create.isPending || (parseFloat(limit) <= 0 && parseInt(limitTokens) <= 0) || (scopeKind === "api_key" && !scopeId)}>
          {create.isPending ? "Creating…" : "Create budget"}
        </Button>
      </div>
    </form>
  );
}

/* ── Edit form ───────────────────────────────────────────────────── */

function EditBudgetForm({ budget, onClose }: { budget: BudgetStatus; onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();

  const [limit, setLimit] = useState(budget.limit_micros > 0 ? (budget.limit_micros / 1_000_000).toFixed(2) : "");
  const [limitTokens, setLimitTokens] = useState(budget.limit_tokens > 0 ? budget.limit_tokens.toString() : "");
  const [period, setPeriod] = useState(budget.period);
  const [alertPct, setAlertPct] = useState(budget.alert_pct);
  const [hardCutoff, setHardCutoff] = useState(budget.hard_cutoff);
  const [error, setError] = useState("");

  const update = useMutation({
    mutationFn: () =>
      api.updateBudget(budget.id, {
        limit_usd: parseFloat(limit) || undefined,
        limit_tokens: parseInt(limitTokens) || undefined,
        period,
        alert_pct: alertPct,
        hard_cutoff: hardCutoff,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["budget-status"] });
      qc.invalidateQueries({ queryKey: ["budgets"] });
      const parts = [];
      if (parseFloat(limit) > 0) parts.push(`$${parseFloat(limit).toFixed(2)}`);
      if (parseInt(limitTokens) > 0) parts.push(`${formatTokens(parseInt(limitTokens))} tokens`);
      toast.success(
        "Budget updated",
        `Limit changed to ${parts.join(" + ")} ${period}. ${hardCutoff ? "Hard cutoff is active." : "Advisory mode — requests won't be blocked."}`,
      );
      onClose();
    },
    onError: (e: Error) => {
      setError(e.message);
      toast.error("Budget update failed", e.message);
    },
  });

  return (
    <form
      className="space-y-4 px-6 py-5"
      onSubmit={(e) => {
        e.preventDefault();
        if (parseFloat(limit) > 0 || parseInt(limitTokens) > 0) update.mutate();
      }}
    >
      {/* Limit (USD) + Limit (Tokens) + Period */}
      <div className="flex gap-3">
        <div className="flex-1">
          <Field label="Limit (USD)">
            <Input
              type="number"
              min="0"
              step="0.01"
              value={limit}
              onChange={(e) => setLimit(e.target.value)}
              placeholder="0 = no limit"
            />
          </Field>
        </div>
        <div className="flex-1">
          <Field label="Limit (Tokens)">
            <FormattedTokenInput
              value={limitTokens}
              onChange={setLimitTokens}
              placeholder="0 = no limit"
            />
          </Field>
        </div>
        <div className="w-36">
          <Field label="Period">
            <Select value={period} onChange={(e) => setPeriod(e.target.value)}>
              {periods.map((p) => (
                <option key={p.value} value={p.value}>
                  {p.label}
                </option>
              ))}
            </Select>
          </Field>
        </div>
      </div>

      <div className="flex items-end gap-6">
        <div className="w-40">
          <Field label="Alert threshold (%)">
            <Input
              type="number"
              min="1"
              max="100"
              value={alertPct}
              onChange={(e) => setAlertPct(parseInt(e.target.value) || 80)}
            />
          </Field>
        </div>
        <div className="flex items-center gap-2 pb-0.5">
          <Toggle checked={hardCutoff} onChange={setHardCutoff} />
          <span className="text-sm">Hard cutoff</span>
        </div>
      </div>

      {error && <ErrorBanner message={error} />}

      <div className="flex gap-2 pt-2 border-t border-[var(--border)]">
        <div className="flex-1" />
        <Button variant="ghost" type="button" onClick={onClose}>
          Cancel
        </Button>
        <Button type="submit" disabled={update.isPending || (parseFloat(limit) <= 0 && parseInt(limitTokens) <= 0)}>
          {update.isPending ? "Saving…" : "Save changes"}
        </Button>
      </div>
    </form>
  );
}

