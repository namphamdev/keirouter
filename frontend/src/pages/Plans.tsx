import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Wallet,
  Plus,
  Pencil,
  Trash2,
  KeyRound,
  ShieldCheck,
  DollarSign,
  Gauge,
} from "lucide-react";
import { api, type Plan } from "../lib/api";
import { FormattedTokenInput, ModelMultiSelect } from "../components/ModelSelect";
import { microsToUSD, formatTokens } from "../lib/format";
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

function parseUSD(value: string): number {
  const n = parseFloat(value);
  return Number.isFinite(n) && n > 0 ? n : 0;
}

function parseTokens(value: string): number {
  const n = parseInt(value, 10);
  return Number.isFinite(n) && n > 0 ? n : 0;
}

function clampAlertPct(value: number): number {
  if (!Number.isFinite(value)) return 80;
  return Math.min(100, Math.max(1, value));
}

export function PlansPage() {
  const qc = useQueryClient();
  const toast = useToast();

  const plans = useQuery({
    queryKey: ["plans"],
    queryFn: () => api.listPlans(),
  });

  const [showCreate, setShowCreate] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);

  const remove = useMutation({
    mutationFn: (id: string) => api.deletePlan(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["plans"] });
      toast.success("Plan deleted", "The plan has been removed.");
    },
    onError: (e: Error) => toast.error("Delete failed", e.message),
  });

  const planList = plans.data?.plans ?? [];
  const editingPlan = planList.find((p) => p.id === editingId);

  return (
    <>
      <PageHeader
        title="Plans"
        icon={Wallet}
        description="Reusable templates for API key budget limits and model restrictions."
        action={
          <Button onClick={() => setShowCreate(true)}>
            <Plus className="h-4 w-4" />
            New plan
          </Button>
        }
      />

      <Modal
        open={showCreate}
        onClose={() => setShowCreate(false)}
        title="Create plan"
        subtitle="Define budget rules and model restrictions as a reusable template."
        maxWidth="max-w-xl"
      >
        <PlanForm onClose={() => setShowCreate(false)} />
      </Modal>

      <Modal
        open={!!editingId}
        onClose={() => setEditingId(null)}
        title="Edit plan"
        subtitle={editingPlan ? `Editing "${editingPlan.name}"` : undefined}
        maxWidth="max-w-xl"
      >
        {editingPlan && (
          <PlanForm
            key={editingPlan.id}
            plan={editingPlan}
            onClose={() => setEditingId(null)}
          />
        )}
      </Modal>

      <Card>
        <SectionHeader
          title="All plans"
          description="API keys inherit rules from their assigned plan."
          icon={ShieldCheck}
        />
        {plans.isLoading ? (
          <div className="px-6 pb-6">
            <Spinner />
          </div>
        ) : planList.length === 0 ? (
          <div className="px-6 pb-6">
            <EmptyState title="No plans yet" hint="Create a plan to use as a template for API keys." />
          </div>
        ) : (
          <div className="divide-y divide-[var(--border)]">
            {planList.map((p) => (
              <PlanRow
                key={p.id}
                plan={p}
                onEdit={() => setEditingId(p.id)}
                onDelete={() => {
                  if (p.key_count > 0) {
                    toast.error("Cannot delete", `This plan has ${p.key_count} key(s) assigned. Reassign them first.`);
                    return;
                  }
                  if (confirm(`Delete plan "${p.name}"?`)) {
                    remove.mutate(p.id);
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

/* ── Plan row ────────────────────────────────────────────────────── */

function PlanRow({
  plan: p,
  onEdit,
  onDelete,
}: {
  plan: Plan;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const hasUSD = p.limit_micros > 0;
  const hasTokens = p.limit_tokens > 0;
  const models = p.allowed_models ?? [];

  return (
    <div className="px-6 py-5">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm font-medium">{p.name}</span>
            <Badge>{p.period}</Badge>
            {p.hard_cutoff ? (
              <Badge tone="danger">hard cutoff</Badge>
            ) : (
              <Badge tone="neutral">advisory</Badge>
            )}
            {models.length > 0 && (
              <Badge tone="accent">{models.length} model{models.length > 1 ? "s" : ""}</Badge>
            )}
          </div>

          {p.description && (
            <p className="mt-1 text-xs text-[var(--text-muted)]">{p.description}</p>
          )}

          <div className="mt-2 flex flex-wrap items-center gap-4 text-xs text-[var(--text-muted)]">
            {hasUSD && (
              <span className="flex items-center gap-1">
                <DollarSign className="h-3 w-3" />
                {microsToUSD(p.limit_micros)} / {p.period}
              </span>
            )}
            {hasTokens && (
              <span className="flex items-center gap-1">
                <Gauge className="h-3 w-3" />
                {formatTokens(p.limit_tokens)} tokens / {p.period}
              </span>
            )}
            {!hasUSD && !hasTokens && (
              <span className="flex items-center gap-1">
                <DollarSign className="h-3 w-3" />
                No spend limit
              </span>
            )}
            <span className="flex items-center gap-1">
              <KeyRound className="h-3 w-3" />
              {p.key_count} key{p.key_count !== 1 ? "s" : ""}
            </span>
            <span>Alert at {p.alert_pct}%</span>
          </div>

          {models.length > 0 && (
            <p className="mt-1.5 text-xs text-[var(--text-muted)]">
              Models: {models.join(", ")}
            </p>
          )}
        </div>

        <div className="flex shrink-0 items-center gap-1.5">
          <Button variant="ghost" onClick={onEdit} className="px-2" title="Edit plan">
            <Pencil className="h-4 w-4" />
          </Button>
          <Button variant="danger" onClick={onDelete} className="px-2" title="Delete plan">
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  );
}

/* ── Plan form (create / edit) ───────────────────────────────────── */

function PlanForm({ plan, onClose }: { plan?: Plan; onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();

  const isEdit = !!plan;

  const [name, setName] = useState(plan?.name ?? "");
  const [description, setDescription] = useState(plan?.description ?? "");
  const [limit, setLimit] = useState(
    plan && plan.limit_micros > 0 ? (plan.limit_micros / 1_000_000).toFixed(2) : ""
  );
  const [limitTokens, setLimitTokens] = useState(
    plan && plan.limit_tokens > 0 ? plan.limit_tokens.toString() : ""
  );
  const [period, setPeriod] = useState(plan?.period ?? "monthly");
  const [alertPct, setAlertPct] = useState(plan?.alert_pct ?? 80);
  const [hardCutoff, setHardCutoff] = useState(plan?.hard_cutoff ?? true);
  const [allowedModels, setAllowedModels] = useState<string[]>(plan?.allowed_models ?? []);
  const [error, setError] = useState("");

  const usdLimit = parseUSD(limit);
  const tokenLimit = parseTokens(limitTokens);
  const canSubmit = name.trim().length > 0;

  const create = useMutation({
    mutationFn: () =>
      api.createPlan({
        name: name.trim(),
        description: description.trim() || undefined,
        limit_usd: usdLimit > 0 ? usdLimit : undefined,
        limit_tokens: tokenLimit > 0 ? tokenLimit : undefined,
        period,
        alert_pct: alertPct,
        hard_cutoff: hardCutoff,
        allowed_models: allowedModels.length > 0 ? allowedModels : undefined,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["plans"] });
      toast.success("Plan created", `"${name}" is ready to assign to API keys.`);
      onClose();
    },
    onError: (e: Error) => {
      setError(e.message);
      toast.error("Plan creation failed", e.message);
    },
  });

  const update = useMutation({
    mutationFn: () =>
      api.updatePlan(plan!.id, {
        name: name.trim(),
        description: description.trim() || undefined,
        limit_usd: usdLimit > 0 ? usdLimit : undefined,
        limit_tokens: tokenLimit > 0 ? tokenLimit : undefined,
        period,
        alert_pct: alertPct,
        hard_cutoff: hardCutoff,
        allowed_models: allowedModels.length > 0 ? allowedModels : [],
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["plans"] });
      toast.success("Plan updated", `"${name}" has been updated.`);
      onClose();
    },
    onError: (e: Error) => {
      setError(e.message);
      toast.error("Plan update failed", e.message);
    },
  });

  const isPending = create.isPending || update.isPending;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    if (!canSubmit) return;
    if (isEdit) {
      update.mutate();
    } else {
      create.mutate();
    }
  };

  return (
    <form
      className="flex flex-col max-h-[calc(100vh-10rem)]"
      onSubmit={handleSubmit}
    >
      <div className="px-5 py-4 space-y-6 overflow-y-auto min-h-0">
        {/* Name & Description */}
        <section className="space-y-3">
          <div>
            <h3 className="text-sm font-semibold text-[var(--text-strong)]">Plan Details</h3>
            <p className="text-xs text-[var(--text-muted)]">Name this plan so you can identify it when assigning to keys.</p>
          </div>
          <Field label="Name">
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Pro"
              autoFocus
            />
          </Field>
          <Field label="Description (optional)">
            <Input
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Full access with $100/month budget"
            />
          </Field>
        </section>

        <div className="h-px bg-[var(--border)] w-full" />

        {/* Budget limits */}
        <section className="space-y-3">
          <div>
            <h3 className="text-sm font-semibold text-[var(--text-strong)]">Budget Limits</h3>
            <p className="text-xs text-[var(--text-muted)]">Keys assigned to this plan inherit these limits.</p>
          </div>
          <div className="grid gap-3 sm:grid-cols-3">
            <Field label="Limit (USD)">
              <div className="relative">
                <div className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3">
                  <DollarSign className="h-4 w-4 text-[var(--text-muted)]" />
                </div>
                <Input
                  type="number"
                  min="0"
                  step="0.01"
                  value={limit}
                  onChange={(e) => setLimit(e.target.value)}
                  placeholder="50.00"
                  className="pl-9"
                />
              </div>
            </Field>
            <Field label="Limit (Tokens)">
              <FormattedTokenInput
                value={limitTokens}
                onChange={setLimitTokens}
                placeholder="100000000"
              />
            </Field>
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
        </section>

        <div className="h-px bg-[var(--border)] w-full" />

        {/* Allowed models */}
        <section className="space-y-3">
          <div>
            <h3 className="text-sm font-semibold text-[var(--text-strong)]">Model Restrictions</h3>
            <p className="text-xs text-[var(--text-muted)]">Leave empty to allow all models. Keys can override this.</p>
          </div>
          <Field label="Allowed models">
            <ModelMultiSelect value={allowedModels} onChange={setAllowedModels} />
            <p className="mt-1 text-[10px] text-[var(--text-muted)]">
              Select models or add custom patterns with * wildcard (e.g. claude-*)
            </p>
          </Field>
        </section>

        <div className="h-px bg-[var(--border)] w-full" />

        {/* Enforcement */}
        <section className="space-y-3">
          <div>
            <h3 className="text-sm font-semibold text-[var(--text-strong)]">Enforcement</h3>
            <p className="text-xs text-[var(--text-muted)]">How to handle budget exhaustion.</p>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Alert threshold (%)">
              <div className="relative">
                <Input
                  type="number"
                  min="1"
                  max="100"
                  value={alertPct}
                  onChange={(e) => setAlertPct(clampAlertPct(parseInt(e.target.value, 10)))}
                  className="pr-8"
                />
                <div className="pointer-events-none absolute inset-y-0 right-0 flex items-center pr-3">
                  <span className="text-sm text-[var(--text-muted)]">%</span>
                </div>
              </div>
            </Field>
            <div className="flex items-center justify-between gap-4 rounded-xl border border-[var(--border)] bg-[var(--bg-subtle)] px-4 py-3">
              <div className="min-w-0">
                <p className="text-sm font-medium">Hard cutoff</p>
                <p className="mt-0.5 text-xs text-[var(--text-muted)]">
                  {hardCutoff ? "Block requests when exhausted." : "Track usage only."}
                </p>
              </div>
              <Toggle checked={hardCutoff} onChange={setHardCutoff} />
            </div>
          </div>
        </section>

        {error && <ErrorBanner message={error} />}
      </div>

      <div className="shrink-0 flex gap-3 border-t border-[var(--border)] bg-[var(--bg-subtle)] px-6 py-4 rounded-b-xl">
        <div className="flex-1" />
        <Button variant="ghost" type="button" onClick={onClose}>
          Cancel
        </Button>
        <Button type="submit" disabled={isPending || !canSubmit}>
          {isPending ? "Saving…" : isEdit ? "Save changes" : "Create plan"}
        </Button>
      </div>
    </form>
  );
}