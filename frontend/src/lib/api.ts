// Typed client for the KeiRouter admin API. All calls go through the dev-server
// proxy (or the embedded static server in production) to /api.

export interface Provider {
  id: string;
  display_name: string;
  dialect: string;
  auth_kind: string;
  input_per_m: number;
  output_per_m: number;
}

export interface APIKey {
  id: string;
  name: string;
  display: string;
  disabled: boolean;
  created_at: string;
}

export interface CreatedKey {
  id: string;
  name: string;
  key: string;
  display: string;
}

export interface Account {
  id: string;
  provider: string;
  label: string;
  auth_kind: string;
  priority: number;
  disabled: boolean;
  created_at: string;
}

export interface ChainStep {
  provider: string;
  model: string;
  position: number;
}

export interface Chain {
  id: string;
  name: string;
  strategy: string;
  steps: ChainStep[];
}

export interface Budget {
  id: string;
  scope_kind: string;
  scope_id: string;
  limit_micros: number;
  period: string;
  alert_pct: number;
  hard_cutoff: boolean;
}

export interface UsageSummary {
  total_requests: number;
  prompt_tokens: number;
  completion_tokens: number;
  cached_tokens: number;
  cost_usd: number;
  cache_hits: number;
  since: string;
}

class APIError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`/api${path}`, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    let message = res.statusText;
    try {
      const data = await res.json();
      message = data?.error?.message ?? message;
    } catch {
      // keep statusText
    }
    throw new APIError(res.status, message);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export const api = {
  providers: () => request<{ providers: Provider[] }>("GET", "/providers"),

  listKeys: () => request<{ keys: APIKey[] }>("GET", "/keys"),
  createKey: (name: string) => request<CreatedKey>("POST", "/keys", { name }),
  deleteKey: (id: string) => request<void>("DELETE", `/keys/${id}`),

  listAccounts: () => request<{ accounts: Account[] }>("GET", "/accounts"),
  createAccount: (input: { provider: string; label: string; api_key: string; base_url?: string }) =>
    request<{ id: string }>("POST", "/accounts", input),
  deleteAccount: (id: string) => request<void>("DELETE", `/accounts/${id}`),

  listChains: () => request<{ chains: Chain[] }>("GET", "/chains"),
  createChain: (input: { name: string; steps: { provider: string; model: string }[] }) =>
    request<{ id: string }>("POST", "/chains", input),
  deleteChain: (id: string) => request<void>("DELETE", `/chains/${id}`),

  listBudgets: () => request<{ budgets: Budget[] }>("GET", "/budgets"),
  createBudget: (input: { scope_kind?: string; limit_usd: number; period?: string }) =>
    request<{ id: string }>("POST", "/budgets", input),
  deleteBudget: (id: string) => request<void>("DELETE", `/budgets/${id}`),

  usage: (period: string) => request<UsageSummary>("GET", `/usage?period=${period}`),
};

export { APIError };