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

function progressColor(pct: number, alertPct: number): string {
  if (pct >= 100) return "bg-red-500";
  if (pct >= alertPct) return "bg-amber-500";
  if (pct >= alertPct * 0.75) return "bg-amber-400";
  return "bg-emerald-500";
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

  return (
    <>
      <PageHeader
        title="Budgets"
        icon={Wallet}
        description="Hard spend caps per key or tenant. Requests are auto-blocked when a budget is exhausted."
        action={
          <Button onClick={() => setShowCreate((v) => !v)}>
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
                  .filter((b) => b.pct_used >= 100 && b.hard_cutoff)
                  .map((b) => `${b.scope_name} (${b.period})`)
                  .join(", ")}
              </p>
            </div>
          </div>
        </Card>
      )}

      {budgets.some((b) => b.pct_used >= b.alert_pct && b.pct_used < 100) && (
        <Card className="mb-6 border-amber-300 bg-amber-50 dark:border-amber-800 dark:bg-amber-950/30">
          <div className="flex items-center gap-3 px-6 py-4">
            <AlertTriangle className="h-5 w-5 shrink-0 text-amber-600 dark:text-amber-400" />
            <div>
              <p className="text-sm font-medium text-amber-800 dark:text-amber-200">
                Budget alert threshold reached
              </p>
              <p className="text-xs text-amber-600 dark:text-amber-400">
                {budgets
                  .filter((b) => b.pct_used >= b.alert_pct && b.pct_used < 100)
                  .map((b) => `${b.scope_name}: ${b.pct_used.toFixed(0)}% used`)
                  .join(", ")}
              </p>
            </div>
          </div>
        </Card>
      )}

      {/* ── Create form ────────────────────────────────────────── */}
      {showCreate && (
        <CreateBudgetForm
          keys={keys.data?.keys ?? []}
          onClose={() => setShowCreate(false)}
        />
      )}

      {/* ── Edit form ──────────────────────────────────────────── */}
      {editingId && budgets.find((b) => b.id === editingId) && (
        <EditBudgetForm
          budget={budgets.find((b) => b.id === editingId)!}
          onClose={() => setEditingId(null)}
        />
      )}

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
            <span className="text-sm font-medium">{microsToUSD(b.limit_micros)}</span>
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

          {/* Progress bar */}
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
              {/* Alert threshold marker */}
              <div
                className="absolute top-0 bottom-0 w-px bg-amber-400/60 z-10"
                style={{ left: `${Math.min(b.alert_pct, 100)}%` }}
                title={`Alert at ${b.alert_pct}%`}
              />
              {/* Spend bar */}
              <div
                className={`h-full rounded-full transition-all duration-700 ${progressColor(b.pct_used, b.alert_pct)}`}
                style={{ width: `${Math.min(b.pct_used, 100)}%` }}
              />
            </div>
            <div className="mt-1.5 flex items-center justify-between text-xs text-[var(--text-muted)]">
              <span>
                {microsToUSD(remaining)} remaining
              </span>
              <span>
                alert at {b.alert_pct}%
              </span>
            </div>
          </div>
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
  const [period, setPeriod] = useState("monthly");
  const [alertPct, setAlertPct] = useState(80);
  const [hardCutoff, setHardCutoff] = useState(true);
  const [error, setError] = useState("");

  const create = useMutation({
    mutationFn: () =>
      api.createBudget({
        scope_kind: scopeKind,
        scope_id: scopeKind === "api_key" && scopeId ? scopeId : undefined,
        limit_usd: parseFloat(limit),
        period,
        alert_pct: alertPct,
        hard_cutoff: hardCutoff,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["budget-status"] });
      qc.invalidateQueries({ queryKey: ["budgets"] });
      toast.success(
        "Budget created",
        `$${parseFloat(limit).toFixed(2)} ${period} limit set for ${scopeKind === "api_key" ? "API key" : "tenant"}.${hardCutoff ? " Requests will be blocked when exhausted." : ""}`,
      );
      onClose();
    },
    onError: (e: Error) => {
      setError(e.message);
      toast.error("Budget creation failed", e.message);
    },
  });

  return (
    <Card className="mb-6">
      <SectionHeader title="Create budget" description="Set a spending cap for a scope and period." icon={Plus} />
      <form
        className="space-y-4 px-6 pb-6"
        onSubmit={(e) => {
          e.preventDefault();
          if (parseFloat(limit) > 0) create.mutate();
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

        {/* Limit + Period */}
        <div className="flex gap-3">
          <div className="w-40">
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
          <div className="w-40">
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

        <div className="flex gap-2 pt-1">
          <Button type="submit" disabled={create.isPending || parseFloat(limit) <= 0 || (scopeKind === "api_key" && !scopeId)}>
            <Plus className="h-4 w-4" />
            {create.isPending ? "Creating…" : "Create budget"}
          </Button>
          <Button variant="ghost" type="button" onClick={onClose}>
            Cancel
          </Button>
        </div>
      </form>
    </Card>
  );
}

/* ── Edit form ───────────────────────────────────────────────────── */

function EditBudgetForm({ budget, onClose }: { budget: BudgetStatus; onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();

  const [limit, setLimit] = useState((budget.limit_micros / 1_000_000).toFixed(2));
  const [period, setPeriod] = useState(budget.period);
  const [alertPct, setAlertPct] = useState(budget.alert_pct);
  const [hardCutoff, setHardCutoff] = useState(budget.hard_cutoff);
  const [error, setError] = useState("");

  const update = useMutation({
    mutationFn: () =>
      api.updateBudget(budget.id, {
        limit_usd: parseFloat(limit),
        period,
        alert_pct: alertPct,
        hard_cutoff: hardCutoff,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["budget-status"] });
      qc.invalidateQueries({ queryKey: ["budgets"] });
      toast.success(
        "Budget updated",
        `Limit changed to $${parseFloat(limit).toFixed(2)} ${period}. ${hardCutoff ? "Hard cutoff is active." : "Advisory mode — requests won't be blocked."}`,
      );
      onClose();
    },
    onError: (e: Error) => {
      setError(e.message);
      toast.error("Budget update failed", e.message);
    },
  });

  return (
    <Card className="mb-6">
      <SectionHeader
        title="Edit budget"
        description={`Editing ${budget.scope_name} ${budget.period} budget`}
        icon={Pencil}
      />
      <form
        className="space-y-4 px-6 pb-6"
        onSubmit={(e) => {
          e.preventDefault();
          if (parseFloat(limit) > 0) update.mutate();
        }}
      >
        <div className="flex gap-3">
          <div className="w-40">
            <Field label="Limit (USD)">
              <Input
                type="number"
                min="0"
                step="0.01"
                value={limit}
                onChange={(e) => setLimit(e.target.value)}
              />
            </Field>
          </div>
          <div className="w-40">
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

        <div className="flex gap-2 pt-1">
          <Button type="submit" disabled={update.isPending || parseFloat(limit) <= 0}>
            {update.isPending ? "Saving…" : "Save changes"}
          </Button>
          <Button variant="ghost" type="button" onClick={onClose}>
            Cancel
          </Button>
        </div>
      </form>
    </Card>
  );
}
