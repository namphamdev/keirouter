import { NavLink, Outlet } from "react-router-dom";

const nav = [
  { to: "/", label: "Overview", end: true },
  { to: "/providers", label: "Providers" },
  { to: "/accounts", label: "Accounts" },
  { to: "/chains", label: "Routing Chains" },
  { to: "/keys", label: "API Keys" },
  { to: "/budgets", label: "Budgets" },
];

export function Layout() {
  return (
    <div className="flex h-full">
      <aside className="flex w-56 shrink-0 flex-col border-r border-[var(--border)] bg-[var(--bg-elevated)]">
        <div className="flex items-center gap-2 px-5 py-5">
          <div className="flex h-7 w-7 items-center justify-center rounded-md bg-accent-600 text-sm font-bold text-white">
            K
          </div>
          <span className="text-sm font-semibold tracking-tight">KeiRouter</span>
        </div>
        <nav className="flex-1 space-y-0.5 px-3">
          {nav.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                `block rounded-md px-3 py-2 text-sm transition-colors ${
                  isActive
                    ? "bg-accent-100 font-medium text-accent-700 dark:bg-accent-800/30 dark:text-accent-200"
                    : "text-[var(--text-muted)] hover:bg-ink-100 hover:text-[var(--text)] dark:hover:bg-ink-800"
                }`
              }
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="px-5 py-4 text-xs text-[var(--text-muted)]">
          <span className="font-mono">localhost:20180/v1</span>
        </div>
      </aside>

      <main className="flex-1 overflow-y-auto">
        <div className="mx-auto max-w-5xl px-8 py-8">
          <Outlet />
        </div>
      </main>
    </div>
  );
}

export function PageHeader({ title, description }: { title: string; description?: string }) {
  return (
    <header className="mb-6">
      <h1 className="text-lg font-semibold tracking-tight">{title}</h1>
      {description && <p className="mt-1 text-sm text-[var(--text-muted)]">{description}</p>}
    </header>
  );
}