// Reusable UI primitives styled with the KeiRouter design system. Calm,
// generously spaced, soft borders and shadows — no gradients or neon.
import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode, SelectHTMLAttributes } from "react";

export function Card({ children, className = "" }: { children: ReactNode; className?: string }) {
  return (
    <div
      className={`rounded-[var(--radius-card)] border border-[var(--border)] bg-[var(--bg-elevated)] ${className}`}
    >
      {children}
    </div>
  );
}

export function CardHeader({ title, description, action }: { title: string; description?: string; action?: ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-4 border-b border-[var(--border)] px-5 py-4">
      <div>
        <h2 className="text-sm font-semibold tracking-tight">{title}</h2>
        {description && <p className="mt-0.5 text-xs text-[var(--text-muted)]">{description}</p>}
      </div>
      {action}
    </div>
  );
}

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "ghost" | "danger";
};

export function Button({ variant = "primary", className = "", ...props }: ButtonProps) {
  const base =
    "inline-flex items-center justify-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/60";
  const variants = {
    primary: "bg-accent-600 text-white hover:bg-accent-700",
    ghost: "border border-[var(--border)] text-[var(--text)] hover:bg-ink-100 dark:hover:bg-ink-800",
    danger: "text-[color:var(--color-danger)] hover:bg-[color:var(--color-danger)]/10",
  };
  return <button className={`${base} ${variants[variant]} ${className}`} {...props} />;
}

export function Input({ className = "", ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={`w-full rounded-md border border-[var(--border)] bg-transparent px-3 py-1.5 text-sm placeholder:text-[var(--text-muted)] focus:border-accent-400 focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/40 ${className}`}
      {...props}
    />
  );
}

export function Select({ className = "", children, ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select
      className={`w-full rounded-md border border-[var(--border)] bg-[var(--bg-elevated)] px-3 py-1.5 text-sm focus:border-accent-400 focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/40 ${className}`}
      {...props}
    >
      {children}
    </select>
  );
}

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block space-y-1">
      <span className="text-xs font-medium text-[var(--text-muted)]">{label}</span>
      {children}
    </label>
  );
}

export function Badge({ children, tone = "neutral" }: { children: ReactNode; tone?: "neutral" | "accent" | "danger" }) {
  const tones = {
    neutral: "bg-ink-100 text-ink-600 dark:bg-ink-800 dark:text-ink-300",
    accent: "bg-accent-100 text-accent-700 dark:bg-accent-800/40 dark:text-accent-200",
    danger: "bg-[color:var(--color-danger)]/10 text-[color:var(--color-danger)]",
  };
  return <span className={`inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium ${tones[tone]}`}>{children}</span>;
}

export function EmptyState({ title, hint }: { title: string; hint?: string }) {
  return (
    <div className="px-5 py-12 text-center">
      <p className="text-sm text-[var(--text-muted)]">{title}</p>
      {hint && <p className="mt-1 text-xs text-[var(--text-muted)]">{hint}</p>}
    </div>
  );
}

export function Spinner() {
  return (
    <div className="flex items-center justify-center py-8">
      <div className="h-5 w-5 animate-spin rounded-full border-2 border-ink-300 border-t-accent-500" />
    </div>
  );
}