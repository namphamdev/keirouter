import { useState, useRef, useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { Sparkles, X, ExternalLink, ArrowUpCircle, FileText } from "lucide-react";
import { Link } from "react-router-dom";
import { api } from "../lib/api";
import { ChangelogMarkdown } from "./ChangelogMarkdown";

// useUpdateInfo is a shared hook so the TopBar badge and the Settings page
// read from the same cached query. The check hits GitHub at most every few
// hours (the backend caches), so a long stale time keeps it cheap.
export function useUpdateInfo() {
  return useQuery({
    queryKey: ["update-check"],
    queryFn: () => api.updateCheck(),
    staleTime: 1000 * 60 * 30, // 30 min — backend caches longer anyway
    refetchOnWindowFocus: false,
    retry: false,
  });
}

// UpdateNotification renders a small badge on the TopBar edge. It only appears
// when a newer release is available. Clicking it opens a popover with a short
// changelog preview and a link into the Settings page for the full changelog.
export function UpdateNotification() {
  const { data } = useUpdateInfo();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const onClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, []);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open]);

  // Nothing to show unless GitHub reported a strictly newer version.
  if (!data || !data.update_available) return null;

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="true"
        aria-expanded={open}
        aria-label={`Update available: ${data.latest}`}
        className="relative flex h-11 w-11 items-center justify-center rounded-xl text-accent-600 transition-colors hover:bg-ink-100 focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/60 dark:text-accent-300 dark:hover:bg-ink-800"
      >
        <ArrowUpCircle className="h-5 w-5" strokeWidth={2} />
        {/* Edge dot — signals an available update. */}
        <span className="absolute right-2 top-2 flex h-2.5 w-2.5">
          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-accent-400 opacity-75" />
          <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-accent-500" />
        </span>
      </button>

      {open && (
        <div
          role="dialog"
          aria-label="Update available"
          className="absolute right-0 top-full z-50 mt-2 w-96 overflow-hidden rounded-xl border border-[var(--border)] bg-[var(--bg-elevated)] shadow-[var(--shadow-float)]"
        >
          <div className="flex items-start justify-between gap-2 border-b border-[var(--border)] px-4 py-3">
            <div className="flex items-center gap-2.5">
              <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-accent-100 text-accent-700 dark:bg-accent-800/40 dark:text-accent-200">
                <Sparkles className="h-4 w-4" strokeWidth={2} />
              </div>
              <div>
                <p className="text-sm font-semibold leading-tight">Update available</p>
                <p className="text-xs leading-tight text-[var(--text-muted)]">
                  <span className="font-mono">{data.current}</span>
                  <span className="mx-1">→</span>
                  <span className="font-mono font-medium text-accent-600 dark:text-accent-400">{data.latest}</span>
                </p>
              </div>
            </div>
            <button
              onClick={() => setOpen(false)}
              aria-label="Dismiss"
              className="flex h-7 w-7 items-center justify-center rounded-lg text-[var(--text-muted)] transition-colors hover:bg-ink-100 dark:hover:bg-ink-800"
            >
              <X className="h-4 w-4" />
            </button>
          </div>

          {data.changelog && (
            <div className="max-h-56 overflow-y-auto px-4 py-3">
              <ChangelogMarkdown changelog={data.changelog} compact />
            </div>
          )}

          <div className="flex items-center justify-between gap-2 border-t border-[var(--border)] px-4 py-3">
            <Link
              to="/settings#system"
              onClick={() => setOpen(false)}
              className="flex items-center gap-1.5 rounded-lg bg-accent-50 px-3 py-1.5 text-xs font-medium text-accent-700 transition-colors hover:bg-accent-100 dark:bg-accent-900/30 dark:text-accent-300 dark:hover:bg-accent-900/50"
            >
              <FileText className="h-3.5 w-3.5" />
              View full changelog
            </Link>
            {data.html_url && (
              <a
                href={data.html_url}
                target="_blank"
                rel="noreferrer noopener"
                className="flex items-center gap-1 text-xs text-[var(--text-muted)] transition-colors hover:text-[var(--text)]"
              >
                Release notes
                <ExternalLink className="h-3 w-3" />
              </a>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
