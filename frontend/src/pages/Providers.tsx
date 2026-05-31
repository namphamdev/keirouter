import { useQuery } from "@tanstack/react-query";
import { api } from "../lib/api";
import { PageHeader } from "../components/Layout";
import { Card, CardHeader, Badge, Spinner, EmptyState } from "../components/ui";

export function ProvidersPage() {
  const providers = useQuery({ queryKey: ["providers"], queryFn: () => api.providers() });

  return (
    <>
      <PageHeader
        title="Providers"
        description="Built-in providers you can connect accounts to. Pricing is used for cost estimates."
      />
      <Card>
        <CardHeader title="Available providers" />
        {providers.isLoading ? (
          <Spinner />
        ) : !providers.data?.providers.length ? (
          <EmptyState title="No providers available" />
        ) : (
          <div className="divide-y divide-[var(--border)]">
            {providers.data.providers.map((p) => (
              <div key={p.id} className="flex items-center justify-between px-5 py-3">
                <div>
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium">{p.display_name}</span>
                    <Badge tone="accent">{p.dialect}</Badge>
                  </div>
                  <p className="mt-0.5 font-mono text-xs text-[var(--text-muted)]">{p.id}</p>
                </div>
                <div className="text-right text-xs text-[var(--text-muted)]">
                  {p.input_per_m || p.output_per_m ? (
                    <span>
                      ${p.input_per_m}/${p.output_per_m} per 1M
                    </span>
                  ) : (
                    <span>custom pricing</span>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>
    </>
  );
}