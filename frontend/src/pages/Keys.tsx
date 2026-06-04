import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { KeyRound, Plus, Copy, Check, ToggleLeft, ToggleRight, ChevronDown } from "lucide-react";
import { api, type CreatedKey } from "../lib/api";
import { PageHeader } from "../components/Layout";
import { useToast } from "../components/Toast";
import { Card, SectionHeader, CardHeader, Button, Input, Select, Field, Badge, Spinner, EmptyState, Toggle } from "../components/ui";

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

  const [name, setName] = useState("");
  const [created, setCreated] = useState<CreatedKey | null>(null);
  const [copied, setCopied] = useState(false);

  const [budgetOpen, setBudgetOpen] = useState(false);
  const [budgetLimit, setBudgetLimit] = useState("");
  const [budgetPeriod, setBudgetPeriod] = useState("monthly");
  const [budgetAlertPct, setBudgetAlertPct] = useState(80);
  const [budgetHardCutoff, setBudgetHardCutoff] = useState(true);

  const create = useMutation({
    mutationFn: () => {
      const budget = budgetOpen && parseFloat(budgetLimit) > 0
        ? { budget_limit_usd: parseFloat(budgetLimit), budget_period: budgetPeriod, budget_alert_pct: budgetAlertPct, budget_hard_cutoff: budgetHardCutoff }
        : undefined;
      return api.createKey(name, budget);
    },
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ["keys"] });
      qc.invalidateQueries({ queryKey: ["budgets"] });
      qc.invalidateQueries({ queryKey: ["budget-status"] });
      setCreated(data);
      setName("");
      setBudgetOpen(false);
      setBudgetLimit("");
      const budgetMsg = data.budget
        ? ` A $${(data.budget.limit_micros / 1_000_000).toFixed(2)} ${data.budget.period} budget has been attached.`
        : "";
      toast.success("Key created", `Copy the key below — it won't be shown again.${budgetMsg}`);
    },
    onError: (e: Error) => toast.error("Key creation failed", e.message),
  });

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
      />

      {created && (
        <Card className="mb-6 border-accent-300 dark:border-accent-700">
          <div className="p-6">
            <p className="text-sm font-medium">Copy your new key now — it won't be shown again.</p>
            <div className="mt-3 flex items-center gap-2">
              <code className="flex-1 overflow-x-auto rounded-lg bg-[var(--bg-subtle)] px-3 py-2.5 font-mono text-sm">
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
              <Button
                variant="ghost"
                onClick={() => {
                  setCreated(null);
                  setCopied(false);
                }}
              >
                Done
              </Button>
            </div>
            {created.budget && (
              <p className="mt-2 text-xs text-[var(--text-muted)]">
                Budget: ${(created.budget.limit_micros / 1_000_000).toFixed(2)} / {created.budget.period}
                {created.budget.hard_cutoff ? " (hard cutoff)" : ""}
              </p>
            )}
          </div>
        </Card>
      )}

      <Card className="mb-6">
        <SectionHeader title="Create key" description="Generate a new API key for a tool or device." icon={Plus} />
        <form
          className="space-y-4 px-6 pb-6"
          onSubmit={(e) => {
            e.preventDefault();
            if (name.trim()) create.mutate();
          }}
        >
          <div className="flex items-end gap-3">
            <div className="flex-1">
              <Field label="Key name">
                <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="laptop" />
              </Field>
            </div>
            <Button type="submit" disabled={create.isPending || !name.trim()}>
              <Plus className="h-4 w-4" />
              {create.isPending ? "Creating…" : "Create key"}
            </Button>
          </div>

          {/* Collapsible budget section */}
          <div>
            <button
              type="button"
              onClick={() => setBudgetOpen(!budgetOpen)}
              className="flex items-center gap-1.5 text-sm text-[var(--text-muted)] hover:text-[var(--text)] transition-colors"
            >
              <ChevronDown className={`h-4 w-4 transition-transform ${budgetOpen ? "rotate-180" : ""}`} />
              Budget limits
            </button>

            {budgetOpen && (
              <div className="mt-3 space-y-3 rounded-lg border border-[var(--border)] bg-[var(--bg-subtle)] p-4">
                <div className="flex gap-3">
                  <div className="w-40">
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
                  <div className="w-40">
                    <Field label="Period">
                      <Select value={budgetPeriod} onChange={(e) => setBudgetPeriod(e.target.value)}>
                        {budgetPeriods.map((p) => (
                          <option key={p.value} value={p.value}>{p.label}</option>
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
              </div>
            )}
          </div>
        </form>
      </Card>

      <Card>
        <CardHeader title="Keys" />
        {keys.isLoading ? (
          <Spinner />
        ) : !keys.data?.keys?.length ? (
          <EmptyState title="No keys yet" />
        ) : (
          <div className="divide-y divide-[var(--border)]">
            {keys.data.keys.map((k) => (
              <div key={k.id} className="flex items-center justify-between px-6 py-4">
                <div>
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium">{k.name}</span>
                    {k.disabled ? <Badge tone="danger">disabled</Badge> : <Badge tone="success">active</Badge>}
                  </div>
                  <button
                    type="button"
                    onClick={() => {
                      navigator.clipboard.writeText(k.display);
                      toast.success("Key copied", "The masked key identifier has been copied to your clipboard.");
                    }}
                    className="mt-0.5 flex items-center gap-1.5 font-mono text-xs text-[var(--text-muted)] transition-colors hover:text-[var(--text)]"
                    title="Copy key"
                  >
                    {k.display}
                    <Copy className="h-3 w-3 opacity-50 transition-opacity hover:opacity-100" />
                  </button>
                </div>
                <div className="flex items-center gap-2">
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