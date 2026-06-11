import { useState, useCallback } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { KeyRound, Plus, Copy, Check, ToggleLeft, ToggleRight, ArrowLeft, ArrowRight, Trash2, Wallet, Wrench, DollarSign, Gauge } from "lucide-react";
import { api, type CreatedKey, type Plan } from "../lib/api";
import { microsToUSD, formatTokens } from "../lib/format";
import { PageHeader } from "../components/Layout";
import { useToast } from "../components/Toast";
import { formatTokenLimit, FormattedTokenInput, ModelMultiSelect } from "../components/ModelSelect";
import {
  Card,
  CardHeader,
  Button,
  Input,
  Select,
  Field,
  Badge,
  Spinner,
  EmptyState,
  Toggle,
  Modal,
} from "../components/ui";

const budgetPeriods = [
  { value: "daily", label: "Daily" },
  { value: "weekly", label: "Weekly" },
  { value: "monthly", label: "Monthly" },
  { value: "total", label: "All time" },
];


export function KeysPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const keys = useQuery({ queryKey: ["keys"], queryFn: () => api.listKeys() });

  const [modalOpen, setModalOpen] = useState(false);
  const [step, setStep] = useState<1 | 2 | 3 | 4>(1);

  // Step 1 — name
  const [name, setName] = useState("");

  // Step 2 — budget
  const [budgetLimit, setBudgetLimit] = useState("");
  const [budgetLimitTokens, setBudgetLimitTokens] = useState("");
  const [budgetPeriod, setBudgetPeriod] = useState("monthly");
  const [budgetAlertPct, setBudgetAlertPct] = useState(80);
  const [budgetHardCutoff, setBudgetHardCutoff] = useState(true);
  const [allowedModels, setAllowedModels] = useState<string[]>([]);

  // Step 3 — result
  const [created, setCreated] = useState<CreatedKey | null>(null);
  const [copied, setCopied] = useState(false);

  const openModal = () => {
    setName("");
    setSelectedPlanId("custom");
    setCustomizePlan(false);
    setBudgetLimit("");
    setBudgetLimitTokens("");
    setBudgetPeriod("monthly");
    setBudgetAlertPct(80);
    setBudgetHardCutoff(true);
    setAllowedModels([]);
    setCreated(null);
    setCopied(false);
    setStep(1);
    setModalOpen(true);
  };

  const closeModal = () => {
    setModalOpen(false);
    if (created) {
      setCreated(null);
      setCopied(false);
    }
  };

  // Plan selection
  const plans = useQuery({ queryKey: ["plans"], queryFn: () => api.listPlans() });
  const [selectedPlanId, setSelectedPlanId] = useState<string>("custom");
  const [customizePlan, setCustomizePlan] = useState(false);

  const create = useMutation({
    mutationFn: () => {
      const isCustom = selectedPlanId === "custom";
      const hasLimit = parseFloat(budgetLimit) > 0;
      const hasTokenLimit = parseInt(budgetLimitTokens) > 0;

      const opts: Record<string, unknown> = {};
      if (!isCustom && selectedPlanId) {
        opts.plan_id = selectedPlanId;
      }
      if (isCustom || customizePlan) {
        if (hasLimit) opts.budget_limit_usd = parseFloat(budgetLimit);
        if (hasTokenLimit) opts.budget_limit_tokens = parseInt(budgetLimitTokens);
        if (hasLimit || hasTokenLimit) {
          opts.budget_period = budgetPeriod;
          opts.budget_alert_pct = budgetAlertPct;
          opts.budget_hard_cutoff = budgetHardCutoff;
        }
        if (allowedModels.length > 0) opts.allowed_models = allowedModels;
      }
      return api.createKey(name, Object.keys(opts).length > 0 ? opts as any : undefined);
    },
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ["keys"] });
      qc.invalidateQueries({ queryKey: ["budgets"] });
      qc.invalidateQueries({ queryKey: ["budget-status"] });
      qc.invalidateQueries({ queryKey: ["plans"] });
      setCreated(data);
      setStep(4);
      const planMsg = data.plan ? ` Plan: ${data.plan.name}.` : "";
      const parts = [];
      if (data.budget && data.budget.limit_micros > 0) parts.push(`$${(data.budget.limit_micros / 1_000_000).toFixed(2)}`);
      if (data.budget && data.budget.limit_tokens > 0) parts.push(`${(data.budget.limit_tokens / 1_000_000).toFixed(0)}M tokens`);
      const budgetMsg = parts.length > 0 ? ` Budget: ${parts.join(" + ")} / ${data.budget?.period}.` : "";
      const modelMsg = data.allowed_models?.length ? ` Models: ${data.allowed_models.join(", ")}.` : "";
      toast.success("Key created", `Copy the key below — it won't be shown again.${planMsg}${budgetMsg}${modelMsg}`);
    },
    onError: (e: Error) => toast.error("Key creation failed", e.message),
  });

  // Multi-select state
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());

  const toggleSelect = useCallback((id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const toggleSelectAll = useCallback(() => {
    if (!keys.data?.keys) return;
    setSelectedIds((prev) => {
      if (prev.size === keys.data!.keys!.length) return new Set();
      return new Set(keys.data!.keys!.map((k) => k.id));
    });
  }, [keys.data]);

  const clearSelection = useCallback(() => setSelectedIds(new Set()), []);

  const bulkRemove = useMutation({
    mutationFn: (ids: string[]) => api.deleteKeys(ids),
    onSuccess: (_, ids) => {
      qc.invalidateQueries({ queryKey: ["keys"] });
      clearSelection();
      toast.success(`${ids.length} key${ids.length > 1 ? "s" : ""} revoked`, "All selected keys have been permanently deleted.");
    },
    onError: (e: Error) => toast.error("Bulk revocation failed", e.message),
  });

  const handleBulkDelete = () => {
    const ids = Array.from(selectedIds);
    if (!ids.length) return;
    if (!confirm(`Revoke ${ids.length} key${ids.length > 1 ? "s" : ""}? This cannot be undone.`)) return;
    bulkRemove.mutate(ids);
  };

  const remove = useMutation({
    mutationFn: (id: string) => api.deleteKey(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["keys"] });
      toast.success("Key revoked", "The key has been permanently deleted and can no longer authenticate requests.");
    },
    onError: (e: Error) => toast.error("Revocation failed", e.message),
  });

  const toggleDisabled = useMutation({
    mutationFn: ({ id, disabled }: { id: string; disabled: boolean }) => api.updateKey(id, { disabled }),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ["keys"] });
      toast.success(
        data.disabled ? "Key disabled" : "Key enabled",
        data.disabled
          ? "Requests using this key will be rejected until re-enabled."
          : "This key can now authenticate requests again.",
      );
    },
    onError: (e: Error) => toast.error("Key update failed", e.message),
  });

  return (
    <>
      <PageHeader
        title="API Keys"
        icon={KeyRound}
        description="Keys your tools use to authenticate. Stored hashed; shown once."
        action={
          <Button onClick={openModal}>
            <Plus className="h-4 w-4" />
            New key
          </Button>
        }
      />

      <Modal
        open={modalOpen}
        onClose={closeModal}
        title={step === 4 ? "Key created" : "Create API key"}
        subtitle={
          step === 1
            ? "Name your key so you can identify it later."
            : step === 2
              ? "Choose a plan or set custom limits."
              : step === 3
                ? "Optionally override plan settings for this key."
                : undefined
        }
      >
        {step === 1 && <StepName name={name} setName={setName} onNext={() => setStep(2)} />}
        {step === 2 && (
          <StepPlanSelect
            plans={plans.data?.plans ?? []}
            selectedPlanId={selectedPlanId}
            setSelectedPlanId={setSelectedPlanId}
            onBack={() => setStep(1)}
            onNext={() => setStep(3)}
          />
        )}
        {step === 3 && (
          <StepConfigure
            selectedPlanId={selectedPlanId}
            plans={plans.data?.plans ?? []}
            customizePlan={customizePlan}
            setCustomizePlan={setCustomizePlan}
            budgetLimit={budgetLimit}
            setBudgetLimit={setBudgetLimit}
            budgetLimitTokens={budgetLimitTokens}
            setBudgetLimitTokens={setBudgetLimitTokens}
            budgetPeriod={budgetPeriod}
            setBudgetPeriod={setBudgetPeriod}
            budgetAlertPct={budgetAlertPct}
            setBudgetAlertPct={setBudgetAlertPct}
            budgetHardCutoff={budgetHardCutoff}
            setBudgetHardCutoff={setBudgetHardCutoff}
            allowedModels={allowedModels}
            setAllowedModels={setAllowedModels}
            onBack={() => setStep(2)}
            onCreate={() => create.mutate()}
            isPending={create.isPending}
          />
        )}
        {step === 4 && created && (
          <StepSuccess
            created={created}
            copied={copied}
            setCopied={setCopied}
            onClose={closeModal}
          />
        )}
      </Modal>

      <Card>
        <CardHeader
          title={selectedIds.size > 0 ? `${selectedIds.size} selected` : "Keys"}
          action={
            selectedIds.size > 0 ? (
              <div className="flex items-center gap-2">
                <button
                  onClick={clearSelection}
                  className="rounded-lg px-3 py-1.5 text-xs font-medium text-[var(--text-muted)] transition-colors hover:text-[var(--text)]"
                >
                  Clear
                </button>
                <Button variant="danger" onClick={handleBulkDelete} disabled={bulkRemove.isPending}>
                  <Trash2 className="h-3.5 w-3.5" />
                  Revoke {selectedIds.size}
                </Button>
              </div>
            ) : undefined
          }
        />
        {keys.isLoading ? (
          <Spinner />
        ) : !keys.data?.keys?.length ? (
          <EmptyState title="No keys yet" />
        ) : (
          <div className="divide-y divide-[var(--border)]">
            {/* Select all row */}
            {keys.data.keys.length > 1 && (
              <div className="flex items-center px-6 py-2 bg-[var(--bg-subtle)] border-b border-[var(--border)]">
                <label className="flex items-center gap-4 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={selectedIds.size === keys.data.keys.length}
                    onChange={toggleSelectAll}
                    className="h-3.5 w-3.5 rounded border-[var(--border)] accent-[var(--color-accent)]"
                  />
                  <span className="text-[10px] font-medium uppercase tracking-wider text-[var(--text-muted)]">
                    Select all
                  </span>
                </label>
              </div>
            )}
            {keys.data.keys.map((k) => (
              <div key={k.id} className={`flex items-center gap-4 px-6 py-4 transition-colors ${selectedIds.has(k.id) ? "bg-accent-50/40 dark:bg-accent-950/20" : ""}`}>
                <label className="flex items-center shrink-0 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={selectedIds.has(k.id)}
                    onChange={() => toggleSelect(k.id)}
                    className="h-3.5 w-3.5 rounded border-[var(--border)] accent-[var(--color-accent)]"
                  />
                </label>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium">{k.name}</span>
                    {k.disabled ? <Badge tone="danger">disabled</Badge> : <Badge tone="success">active</Badge>}
                    {k.plan_name && (
                      <Badge tone="neutral">{k.plan_name}</Badge>
                    )}
                    {k.allowed_models && k.allowed_models.length > 0 && (
                      <Badge tone="accent">{k.allowed_models.length} model{k.allowed_models.length > 1 ? "s" : ""}</Badge>
                    )}
                  </div>
                  {k.allowed_models && k.allowed_models.length > 0 && (
                    <p className="mt-0.5 text-xs text-[var(--text-muted)]">
                      Models: {k.allowed_models.join(", ")}
                    </p>
                  )}
                  <div className="flex items-center gap-4 mt-0.5">
                    <button
                      type="button"
                      onClick={() => {
                        navigator.clipboard.writeText(k.display);
                        toast.success("Key copied", "The masked key identifier has been copied to your clipboard.");
                      }}
                      className="flex items-center gap-1.5 font-mono text-xs text-[var(--text-muted)] transition-colors hover:text-[var(--text)]"
                      title="Copy key"
                    >
                      {k.display}
                      <Copy className="h-3 w-3 opacity-50 transition-opacity hover:opacity-100" />
                    </button>
                    
                    <button
                      type="button"
                      onClick={() => {
                        navigator.clipboard.writeText(`${window.location.origin}/portal?id=${k.id}`);
                        toast.success("Portal link copied", "Share this link with the key owner to let them track their usage.");
                      }}
                      className="flex items-center gap-1.5 font-mono text-xs text-emerald-500/80 transition-colors hover:text-emerald-400"
                      title="Copy portal link"
                    >
                      Portal Link
                      <Copy className="h-3 w-3 opacity-50 transition-opacity hover:opacity-100" />
                    </button>
                  </div>
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  <Button
                    variant="ghost"
                    onClick={() => toggleDisabled.mutate({ id: k.id, disabled: !k.disabled })}
                    disabled={toggleDisabled.isPending}
                    className="px-2"
                    title={k.disabled ? "Enable key" : "Disable key"}
                  >
                    {k.disabled ? <ToggleLeft className="h-4 w-4" /> : <ToggleRight className="h-4 w-4" />}
                  </Button>
                  <Button variant="danger" onClick={() => remove.mutate(k.id)}>
                    Revoke
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>
    </>
  );
}

/* ── Step 1: Name ───────────────────────────────────────────────── */

function StepName({
  name,
  setName,
  onNext,
}: {
  name: string;
  setName: (v: string) => void;
  onNext: () => void;
}) {
  return (
    <div className="space-y-4 px-6 py-5">
      <Field label="Key name">
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="laptop"
          autoFocus
          onKeyDown={(e) => {
            if (e.key === "Enter" && name.trim()) {
              e.preventDefault();
              onNext();
            }
          }}
        />
      </Field>
      <div className="flex gap-2 pt-1">
        <Button className="flex-1" onClick={onNext} disabled={!name.trim()}>
          Next
          <ArrowRight className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}

/* ── Step 2: Plan Select ────────────────────────────────────────── */

function StepPlanSelect({
  plans,
  selectedPlanId,
  setSelectedPlanId,
  onBack,
  onNext,
}: {
  plans: Plan[];
  selectedPlanId: string;
  setSelectedPlanId: (v: string) => void;
  onBack: () => void;
  onNext: () => void;
}) {
  return (
    <div className="space-y-4 px-6 py-5">
      <p className="text-xs text-[var(--text-muted)]">
        Select a plan to inherit its budget rules, or choose Custom to set everything yourself.
      </p>

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {plans.map((p) => (
          <button
            key={p.id}
            type="button"
            onClick={() => setSelectedPlanId(p.id)}
            className={`rounded-xl border px-4 py-3 text-left transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/40 ${
              selectedPlanId === p.id
                ? "border-accent-400 bg-accent-500/10"
                : "border-[var(--border)] bg-[var(--bg-subtle)] hover:bg-[var(--bg)]"
            }`}
          >
            <div className="flex items-center gap-2">
              <Wallet className="h-4 w-4 text-[var(--text-muted)]" />
              <span className="text-sm font-medium">{p.name}</span>
            </div>
            <div className="mt-1.5 space-y-0.5 text-xs text-[var(--text-muted)]">
              {p.limit_micros > 0 && <p>{microsToUSD(p.limit_micros)} / {p.period}</p>}
              {p.limit_tokens > 0 && <p>{formatTokens(p.limit_tokens)} tokens / {p.period}</p>}
              {p.limit_micros === 0 && p.limit_tokens === 0 && <p>No spend limit</p>}
              {(p.allowed_models ?? []).length > 0 && <p>{(p.allowed_models ?? []).length} model restriction(s)</p>}
              <p>{p.key_count} key{p.key_count !== 1 ? "s" : ""}</p>
            </div>
          </button>
        ))}

        {/* Custom option */}
        <button
          type="button"
          onClick={() => setSelectedPlanId("custom")}
          className={`rounded-xl border border-dashed px-4 py-3 text-left transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/40 ${
            selectedPlanId === "custom"
              ? "border-accent-400 bg-accent-500/10"
              : "border-[var(--border)] bg-[var(--bg-subtle)] hover:bg-[var(--bg)]"
          }`}
        >
          <div className="flex items-center gap-2">
            <Wrench className="h-4 w-4 text-[var(--text-muted)]" />
            <span className="text-sm font-medium">Custom</span>
          </div>
          <div className="mt-1.5 text-xs text-[var(--text-muted)]">
            <p>No preset</p>
            <p>Set everything yourself</p>
          </div>
        </button>
      </div>

      <div className="flex gap-2 pt-2 border-t border-[var(--border)]">
        <Button variant="ghost" onClick={onBack}>
          <ArrowLeft className="h-4 w-4" />
          Back
        </Button>
        <div className="flex-1" />
        <Button onClick={onNext}>
          Next
          <ArrowRight className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}

/* ── Step 3: Configure (plan details / custom) ──────────────────── */

function StepConfigure({
  selectedPlanId,
  plans,
  customizePlan,
  setCustomizePlan,
  budgetLimit,
  setBudgetLimit,
  budgetLimitTokens,
  setBudgetLimitTokens,
  budgetPeriod,
  setBudgetPeriod,
  budgetAlertPct,
  setBudgetAlertPct,
  budgetHardCutoff,
  setBudgetHardCutoff,
  allowedModels,
  setAllowedModels,
  onBack,
  onCreate,
  isPending,
}: {
  selectedPlanId: string;
  plans: Plan[];
  customizePlan: boolean;
  setCustomizePlan: (v: boolean) => void;
  budgetLimit: string;
  setBudgetLimit: (v: string) => void;
  budgetLimitTokens: string;
  setBudgetLimitTokens: (v: string) => void;
  budgetPeriod: string;
  setBudgetPeriod: (v: string) => void;
  budgetAlertPct: number;
  setBudgetAlertPct: (v: number) => void;
  budgetHardCutoff: boolean;
  setBudgetHardCutoff: (v: boolean) => void;
  allowedModels: string[];
  setAllowedModels: (v: string[]) => void;
  onBack: () => void;
  onCreate: () => void;
  isPending: boolean;
}) {
  const isCustom = selectedPlanId === "custom";
  const selectedPlan = plans.find((p) => p.id === selectedPlanId);
  const models = selectedPlan?.allowed_models ?? [];

  if (isCustom) {
    // Full custom config (same as old StepBudget)
    return (
      <div className="space-y-4 px-6 py-5">
        <div className="flex gap-3">
          <div className="flex-1">
            <Field label="Limit (USD)">
              <Input
                type="number"
                min="0"
                step="0.01"
                value={budgetLimit}
                onChange={(e) => setBudgetLimit(e.target.value)}
                placeholder="50.00"
              />
            </Field>
          </div>
          <div className="flex-1">
            <Field label="Limit (Tokens)">
              <FormattedTokenInput
                value={budgetLimitTokens}
                onChange={setBudgetLimitTokens}
                placeholder="100000000"
              />
            </Field>
          </div>
          <div className="w-36">
            <Field label="Period">
              <Select value={budgetPeriod} onChange={(e) => setBudgetPeriod(e.target.value)}>
                {budgetPeriods.map((p) => (
                  <option key={p.value} value={p.value}>{p.label}</option>
                ))}
              </Select>
            </Field>
          </div>
        </div>

        <Field label="Allowed models">
          <ModelMultiSelect value={allowedModels} onChange={setAllowedModels} />
          <p className="mt-1 text-[10px] text-[var(--text-muted)]">
            Select models or add custom patterns with * wildcard (e.g. claude-*)
          </p>
        </Field>

        <div className="flex items-end gap-6">
          <div className="w-40">
            <Field label="Alert threshold (%)">
              <Input
                type="number"
                min="1"
                max="100"
                value={budgetAlertPct}
                onChange={(e) => setBudgetAlertPct(parseInt(e.target.value) || 80)}
              />
            </Field>
          </div>
          <div className="flex items-center gap-2 pb-0.5">
            <Toggle checked={budgetHardCutoff} onChange={setBudgetHardCutoff} />
            <span className="text-sm">Hard cutoff (block when exhausted)</span>
          </div>
        </div>

        <div className="flex gap-2 pt-2 border-t border-[var(--border)]">
          <Button variant="ghost" onClick={onBack}>
            <ArrowLeft className="h-4 w-4" />
            Back
          </Button>
          <div className="flex-1" />
          <Button variant="ghost" onClick={onCreate} disabled={isPending}>
            {isPending ? "Creating…" : "Skip budget"}
          </Button>
          <Button onClick={onCreate} disabled={isPending}>
            {isPending ? "Creating…" : "Create key"}
          </Button>
        </div>
      </div>
    );
  }

  // Plan selected — show summary + optional override toggle
  return (
    <div className="space-y-4 px-6 py-5">
      {/* Plan summary card */}
      {selectedPlan && (
        <div className="rounded-xl border border-[var(--border)] bg-[var(--bg-subtle)] p-4">
          <div className="flex items-center gap-2">
            <Wallet className="h-4 w-4 text-accent-500" />
            <span className="text-sm font-medium">{selectedPlan.name}</span>
            <Badge>{selectedPlan.period}</Badge>
            {selectedPlan.hard_cutoff ? <Badge tone="danger">hard cutoff</Badge> : <Badge tone="neutral">advisory</Badge>}
          </div>
          {selectedPlan.description && (
            <p className="mt-1 text-xs text-[var(--text-muted)]">{selectedPlan.description}</p>
          )}
          <div className="mt-2 flex flex-wrap items-center gap-3 text-xs text-[var(--text-muted)]">
            {selectedPlan.limit_micros > 0 && (
              <span className="flex items-center gap-1"><DollarSign className="h-3 w-3" />{microsToUSD(selectedPlan.limit_micros)}</span>
            )}
            {selectedPlan.limit_tokens > 0 && (
              <span className="flex items-center gap-1"><Gauge className="h-3 w-3" />{formatTokens(selectedPlan.limit_tokens)} tok</span>
            )}
            {selectedPlan.limit_micros === 0 && selectedPlan.limit_tokens === 0 && <span>No spend limit</span>}
            <span>Alert at {selectedPlan.alert_pct}%</span>
          </div>
          {models.length > 0 && (
            <p className="mt-1.5 text-xs text-[var(--text-muted)]">Models: {models.join(", ")}</p>
          )}
        </div>
      )}

      {/* Override toggle */}
      <div className="rounded-xl border border-[var(--border)] bg-[var(--bg-subtle)] px-4 py-3">
        <div className="flex items-center justify-between gap-4">
          <div>
            <p className="text-sm font-medium">Customize for this key</p>
            <p className="mt-0.5 text-xs text-[var(--text-muted)]">
              Override plan limits with per-key settings.
            </p>
          </div>
          <Toggle checked={customizePlan} onChange={setCustomizePlan} />
        </div>
      </div>

      {/* Override fields */}
      {customizePlan && (
        <div className="space-y-4 rounded-xl border border-[var(--border)] bg-[var(--bg-subtle)] p-4">
          <div className="flex gap-3">
            <div className="flex-1">
              <Field label="Override USD limit">
                <Input
                  type="number"
                  min="0"
                  step="0.01"
                  value={budgetLimit}
                  onChange={(e) => setBudgetLimit(e.target.value)}
                  placeholder="Leave empty to use plan"
                />
              </Field>
            </div>
            <div className="flex-1">
              <Field label="Override token limit">
                <FormattedTokenInput
                  value={budgetLimitTokens}
                  onChange={setBudgetLimitTokens}
                  placeholder="Leave empty to use plan"
                />
              </Field>
            </div>
          </div>
          <Field label="Override allowed models">
            <ModelMultiSelect value={allowedModels} onChange={setAllowedModels} />
            <p className="mt-1 text-[10px] text-[var(--text-muted)]">
              Leave empty to use plan's model restrictions.
            </p>
          </Field>
        </div>
      )}

      <div className="flex gap-2 pt-2 border-t border-[var(--border)]">
        <Button variant="ghost" onClick={onBack}>
          <ArrowLeft className="h-4 w-4" />
          Back
        </Button>
        <div className="flex-1" />
        <Button onClick={onCreate} disabled={isPending}>
          {isPending ? "Creating…" : "Create key"}
        </Button>
      </div>
    </div>
  );
}

/* ── Step 4: Success / Copy ─────────────────────────────────────── */

function StepSuccess({
  created,
  copied,
  setCopied,
  onClose,
}: {
  created: CreatedKey;
  copied: boolean;
  setCopied: (v: boolean) => void;
  onClose: () => void;
}) {
  const [copiedUrl, setCopiedUrl] = useState(false);
  const portalUrl = `${window.location.origin}/portal?id=${created.id}`;

  return (
    <div className="space-y-4 px-6 py-5">
      <div className="rounded-xl border border-[var(--border)] bg-[var(--bg-subtle)] p-4">
        <p className="text-xs font-medium text-[var(--text-muted)]">Your new key — copy it now, it won't be shown again.</p>
        <div className="mt-2 flex items-center gap-2">
          <code className="flex-1 overflow-x-auto rounded-lg bg-[var(--bg-elevated)] px-3 py-2.5 font-mono text-sm">
            {created.key}
          </code>
          <Button
            onClick={() => {
              navigator.clipboard.writeText(created.key);
              setCopied(true);
              setTimeout(() => setCopied(false), 1500);
            }}
          >
            {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
            {copied ? "Copied" : "Copy"}
          </Button>
        </div>
      </div>

      <div className="rounded-xl border border-[var(--border)] bg-[var(--bg-subtle)] p-4">
        <p className="text-xs font-medium text-[var(--text-muted)]">Telemetry Portal URL — share this with the key owner to let them track their usage.</p>
        <div className="mt-2 flex items-center gap-2">
          <code className="flex-1 overflow-x-auto rounded-lg bg-[var(--bg-elevated)] px-3 py-2.5 font-mono text-xs whitespace-nowrap">
            {portalUrl}
          </code>
          <Button
            onClick={() => {
              navigator.clipboard.writeText(portalUrl);
              setCopiedUrl(true);
              setTimeout(() => setCopiedUrl(false), 1500);
            }}
          >
            {copiedUrl ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
            {copiedUrl ? "Copied" : "Copy"}
          </Button>
        </div>
      </div>

      {created.budget && (
        <div className="rounded-lg border border-[var(--border)] bg-[var(--bg-subtle)] px-4 py-3">
          <p className="text-xs font-medium text-[var(--text-muted)]">Plan attached</p>
          <p className="mt-0.5 text-sm">
            {created.budget.limit_micros > 0 && `$${(created.budget.limit_micros / 1_000_000).toFixed(2)}`}
            {created.budget.limit_micros > 0 && created.budget.limit_tokens > 0 && " + "}
            {created.budget.limit_tokens > 0 &&
              `${formatTokenLimit(String(created.budget.limit_tokens))} tokens`}
            {` / ${created.budget.period}`}
            {created.budget.hard_cutoff ? " (hard cutoff)" : ""}
          </p>
        </div>
      )}

      {created.allowed_models && created.allowed_models.length > 0 && (
        <div className="rounded-lg border border-[var(--border)] bg-[var(--bg-subtle)] px-4 py-3">
          <p className="text-xs font-medium text-[var(--text-muted)]">Allowed models</p>
          <p className="mt-0.5 text-sm">{created.allowed_models.join(", ")}</p>
        </div>
      )}

      <Button className="w-full" onClick={onClose}>
        Done
      </Button>
    </div>
  );
}