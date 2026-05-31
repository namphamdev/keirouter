import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../lib/api";
import { PageHeader } from "../components/Layout";
import { Card, CardHeader, Button, Input, Select, Field, Badge, Spinner, EmptyState } from "../components/ui";

interface DraftStep {
  provider: string;
  model: string;
}

export function ChainsPage() {
  const qc = useQueryClient();
  const chains = useQuery({ queryKey: ["chains"], queryFn: () => api.listChains() });
  const providers = useQuery({ queryKey: ["providers"], queryFn: () => api.providers() });

  const [name, setName] = useState("");
  const [steps, setSteps] = useState<DraftStep[]>([{ provider: "", model: "" }]);
  const [error, setError] = useState("");

  const create = useMutation({
    mutationFn: () =>
      api.createChain({ name, steps: steps.filter((s) => s.provider && s.model) }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["chains"] });
      setName("");
      setSteps([{ provider: "", model: "" }]);
      setError("");
    },
    onError: (e: Error) => setError(e.message),
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.deleteChain(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["chains"] }),
  });

  const updateStep = (i: number, patch: Partial<DraftStep>) =>
    setSteps((prev) => prev.map((s, idx) => (idx === i ? { ...s, ...patch } : s)));

  const valid = name.trim() && steps.some((s) => s.provider && s.model);

  return (
    <>
      <PageHeader
        title="Routing Chains"
        description="Ordered fallback. Each request tries steps top to bottom, skipping models that can't honor it."
      />

      <Card className="mb-6">
        <CardHeader title="Create chain" />
        <form
          className="space-y-4 p-5"
          onSubmit={(e) => {
            e.preventDefault();
            if (valid) create.mutate();
          }}
        >
          <Field label="Chain name">
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="coding" />
          </Field>

          <div className="space-y-2">
            <span className="text-xs font-medium text-[var(--text-muted)]">Fallback steps</span>
            {steps.map((step, i) => (
              <div key={i} className="flex items-center gap-2">
                <span className="w-5 text-center text-xs text-[var(--text-muted)]">{i + 1}</span>
                <div className="w-48">
                  <Select value={step.provider} onChange={(e) => updateStep(i, { provider: e.target.value })}>
                    <option value="">Provider…</option>
                    {providers.data?.providers.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.display_name}
                      </option>
                    ))}
                  </Select>
                </div>
                <Input
                  className="flex-1"
                  value={step.model}
                  onChange={(e) => updateStep(i, { model: e.target.value })}
                  placeholder="model id, e.g. gpt-4o"
                />
                {steps.length > 1 && (
                  <Button variant="ghost" type="button" onClick={() => setSteps((s) => s.filter((_, idx) => idx !== i))}>
                    ✕
                  </Button>
                )}
              </div>
            ))}
            <Button variant="ghost" type="button" onClick={() => setSteps((s) => [...s, { provider: "", model: "" }])}>
              + Add step
            </Button>
          </div>

          <div className="flex items-center justify-between">
            {error ? <span className="text-xs text-[color:var(--color-danger)]">{error}</span> : <span />}
            <Button type="submit" disabled={create.isPending || !valid}>
              {create.isPending ? "Creating…" : "Create chain"}
            </Button>
          </div>
        </form>
      </Card>

      <Card>
        <CardHeader title="Chains" />
        {chains.isLoading ? (
          <Spinner />
        ) : !chains.data?.chains.length ? (
          <EmptyState title="No chains yet" hint="Create one above, then target it as chain:name." />
        ) : (
          <div className="divide-y divide-[var(--border)]">
            {chains.data.chains.map((c) => (
              <div key={c.id} className="px-5 py-3">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-sm font-medium">chain:{c.name}</span>
                    <Badge>{c.strategy}</Badge>
                  </div>
                  <Button variant="danger" onClick={() => remove.mutate(c.id)}>
                    Remove
                  </Button>
                </div>
                <div className="mt-2 flex flex-wrap gap-1.5">
                  {c.steps.map((s, i) => (
                    <span key={i} className="font-mono text-xs text-[var(--text-muted)]">
                      {i > 0 && <span className="mr-1.5">→</span>}
                      {s.provider}/{s.model}
                    </span>
                  ))}
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>
    </>
  );
}