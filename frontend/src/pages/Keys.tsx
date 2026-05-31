import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api, type CreatedKey } from "../lib/api";
import { PageHeader } from "../components/Layout";
import { Card, CardHeader, Button, Input, Field, Badge, Spinner, EmptyState } from "../components/ui";

export function KeysPage() {
  const qc = useQueryClient();
  const keys = useQuery({ queryKey: ["keys"], queryFn: () => api.listKeys() });

  const [name, setName] = useState("");
  const [created, setCreated] = useState<CreatedKey | null>(null);

  const create = useMutation({
    mutationFn: () => api.createKey(name),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ["keys"] });
      setCreated(data);
      setName("");
    },
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.deleteKey(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["keys"] }),
  });

  return (
    <>
      <PageHeader title="API Keys" description="Keys your tools use to authenticate. Stored hashed; shown once." />

      {created && (
        <Card className="mb-6 border-accent-300">
          <div className="p-5">
            <p className="text-sm font-medium">Copy your new key now — it won't be shown again.</p>
            <div className="mt-3 flex items-center gap-2">
              <code className="flex-1 overflow-x-auto rounded-md bg-ink-100 px-3 py-2 font-mono text-sm dark:bg-ink-800">
                {created.key}
              </code>
              <Button onClick={() => navigator.clipboard.writeText(created.key)}>Copy</Button>
              <Button variant="ghost" onClick={() => setCreated(null)}>
                Done
              </Button>
            </div>
          </div>
        </Card>
      )}

      <Card className="mb-6">
        <CardHeader title="Create key" />
        <form
          className="flex items-end gap-3 p-5"
          onSubmit={(e) => {
            e.preventDefault();
            if (name.trim()) create.mutate();
          }}
        >
          <div className="flex-1">
            <Field label="Key name">
              <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="laptop" />
            </Field>
          </div>
          <Button type="submit" disabled={create.isPending || !name.trim()}>
            {create.isPending ? "Creating…" : "Create key"}
          </Button>
        </form>
      </Card>

      <Card>
        <CardHeader title="Keys" />
        {keys.isLoading ? (
          <Spinner />
        ) : !keys.data?.keys.length ? (
          <EmptyState title="No keys yet" />
        ) : (
          <div className="divide-y divide-[var(--border)]">
            {keys.data.keys.map((k) => (
              <div key={k.id} className="flex items-center justify-between px-5 py-3">
                <div>
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium">{k.name}</span>
                    {k.disabled && <Badge tone="danger">disabled</Badge>}
                  </div>
                  <p className="mt-0.5 font-mono text-xs text-[var(--text-muted)]">{k.display}</p>
                </div>
                <Button variant="danger" onClick={() => remove.mutate(k.id)}>
                  Revoke
                </Button>
              </div>
            ))}
          </div>
        )}
      </Card>
    </>
  );
}