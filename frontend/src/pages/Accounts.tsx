import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../lib/api";
import { PageHeader } from "../components/Layout";
import { Card, CardHeader, Button, Input, Select, Field, Badge, Spinner, EmptyState } from "../components/ui";

export function AccountsPage() {
  const qc = useQueryClient();
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: () => api.listAccounts() });
  const providers = useQuery({ queryKey: ["providers"], queryFn: () => api.providers() });

  const [provider, setProvider] = useState("");
  const [label, setLabel] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [baseURL, setBaseURL] = useState("");
  const [error, setError] = useState("");

  const create = useMutation({
    mutationFn: () => api.createAccount({ provider, label, api_key: apiKey, base_url: baseURL || undefined }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["accounts"] });
      setLabel("");
      setApiKey("");
      setBaseURL("");
      setError("");
    },
    onError: (e: Error) => setError(e.message),
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.deleteAccount(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["accounts"] }),
  });

  return (
    <>
      <PageHeader title="Accounts" description="Provider credentials. API keys are encrypted at rest and never shown again." />

      <Card className="mb-6">
        <CardHeader title="Add account" />
        <form
          className="grid grid-cols-1 gap-4 p-5 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (provider && apiKey) create.mutate();
          }}
        >
          <Field label="Provider">
            <Select value={provider} onChange={(e) => setProvider(e.target.value)} required>
              <option value="">Select a provider…</option>
              {providers.data?.providers.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.display_name}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Label">
            <Input value={label} onChange={(e) => setLabel(e.target.value)} placeholder="personal" />
          </Field>
          <Field label="API key">
            <Input
              type="password"
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              placeholder="sk-..."
              required
            />
          </Field>
          <Field label="Base URL (optional)">
            <Input value={baseURL} onChange={(e) => setBaseURL(e.target.value)} placeholder="for custom endpoints" />
          </Field>
          <div className="sm:col-span-2 flex items-center justify-between">
            {error ? <span className="text-xs text-[color:var(--color-danger)]">{error}</span> : <span />}
            <Button type="submit" disabled={create.isPending || !provider || !apiKey}>
              {create.isPending ? "Adding…" : "Add account"}
            </Button>
          </div>
        </form>
      </Card>

      <Card>
        <CardHeader title="Connected accounts" />
        {accounts.isLoading ? (
          <Spinner />
        ) : !accounts.data?.accounts.length ? (
          <EmptyState title="No accounts yet" hint="Add a provider account above to start routing." />
        ) : (
          <div className="divide-y divide-[var(--border)]">
            {accounts.data.accounts.map((a) => (
              <div key={a.id} className="flex items-center justify-between px-5 py-3">
                <div>
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium">{a.label || a.provider}</span>
                    <Badge>{a.provider}</Badge>
                    {a.disabled && <Badge tone="danger">disabled</Badge>}
                  </div>
                  <p className="mt-0.5 text-xs text-[var(--text-muted)]">priority {a.priority}</p>
                </div>
                <Button variant="danger" onClick={() => remove.mutate(a.id)}>
                  Remove
                </Button>
              </div>
            ))}
          </div>
        )}
      </Card>
    </>
  );
}