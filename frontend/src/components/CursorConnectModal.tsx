import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { ExternalLink, RefreshCw, X, AlertTriangle, FileUp } from "lucide-react";
import { api, type DeviceCode } from "../lib/api";
import { Button, Input, Field, ErrorBanner } from "./ui";
import { useToast } from "./Toast";
import { Done } from "./KilocodeConnectModal";

// CursorConnectModal supports two ways to connect Cursor:
//   - OAuth login (default): the backend generates a PKCE pair + uuid, opens
//     cursor.com/loginDeepControl in the browser, then polls until the user
//     authorizes and a token pair (with refresh) is returned.
//   - Import token: paste an access token exported from the Cursor IDE.
export function CursorConnectModal({ onClose }: { onClose: () => void }) {
  const [mode, setMode] = useState<"login" | "import">("login");

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4 backdrop-blur-sm"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-labelledby="cursor-modal-title"
    >
      <div
        className="w-full max-w-md rounded-2xl border border-[var(--border)] bg-[var(--bg-elevated)] shadow-[var(--shadow-float)]"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-[var(--border)] px-6 py-4">
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-accent-100 text-accent-700 dark:bg-accent-800/40 dark:text-accent-200">
              {mode === "login" ? <ExternalLink className="h-[18px] w-[18px]" /> : <FileUp className="h-[18px] w-[18px]" />}
            </div>
            <h2 id="cursor-modal-title" className="text-base font-semibold tracking-tight">Connect Cursor</h2>
          </div>
          <button
            onClick={onClose}
            aria-label="Close"
            className="flex h-9 w-9 items-center justify-center rounded-xl text-[var(--text-muted)] transition-colors hover:bg-ink-100 hover:text-[var(--text)] dark:hover:bg-ink-800 focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/60"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="px-6 py-5">
          {mode === "login" ? (
            <LoginFlow onClose={onClose} onUseImport={() => setMode("import")} />
          ) : (
            <ImportFlow onClose={onClose} onUseLogin={() => setMode("login")} />
          )}
        </div>
      </div>
    </div>
  );
}

// LoginFlow drives the deep-control PKCE login: start -> open browser -> poll.
function LoginFlow({ onClose, onUseImport }: { onClose: () => void; onUseImport: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [dc, setDc] = useState<DeviceCode | null>(null);
  const [status, setStatus] = useState<"idle" | "starting" | "waiting" | "done" | "error">("idle");
  const [error, setError] = useState("");
  const [elapsed, setElapsed] = useState(0);
  const pollRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const startedRef = useRef(false);

  useEffect(() => {
    return () => {
      if (pollRef.current) clearTimeout(pollRef.current);
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, []);

  const start = async () => {
    if (startedRef.current) return;
    startedRef.current = true;
    setStatus("starting");
    setError("");

    try {
      const res = await api.cursorLoginStart();
      setDc(res);
      setStatus("waiting");
      timerRef.current = setInterval(() => setElapsed((e) => e + 1), 1000);
      poll(res.device_code, res.interval);
    } catch (e) {
      setError((e as Error).message);
      setStatus("error");
      startedRef.current = false;
      toast.error("Couldn't start Cursor login", (e as Error).message);
    }
  };

  const poll = (deviceCode: string, interval: number) => {
    pollRef.current = setTimeout(async () => {
      try {
        const res = await api.cursorLoginPoll(deviceCode);
        if (res.status === "complete") {
          setStatus("done");
          if (timerRef.current) clearInterval(timerRef.current);
          qc.invalidateQueries({ queryKey: ["accounts"] });
          toast.success("Cursor connected", "Account added successfully.");
          setTimeout(onClose, 1400);
          return;
        }
        poll(deviceCode, res.slow_down ? interval + 5 : interval);
      } catch (e) {
        setError((e as Error).message);
        setStatus("error");
        if (timerRef.current) clearInterval(timerRef.current);
        toast.error("Cursor login failed", (e as Error).message);
      }
    }, Math.max(1, interval) * 1000);
  };

  const formatElapsed = (s: number) => {
    const m = Math.floor(s / 60);
    const sec = s % 60;
    return `${m}:${sec.toString().padStart(2, "0")}`;
  };

  if (status === "done") {
    return <Done provider="Cursor" />;
  }

  if (status === "error") {
    return (
      <div className="space-y-4">
        <ErrorBanner message={error} />
        <div className="flex gap-3">
          <Button variant="ghost" className="flex-1" onClick={start}>
            <RefreshCw className="h-4 w-4" />
            Retry
          </Button>
          <Button variant="ghost" className="flex-1" onClick={onClose}>
            Close
          </Button>
        </div>
      </div>
    );
  }

  if (status === "waiting" && dc) {
    const verificationUrl = dc.verification_uri_complete || dc.verification_uri;
    return (
      <div className="space-y-5">
        <div className="flex items-center justify-between rounded-xl border border-[var(--border)] bg-[var(--bg-subtle)] px-4 py-3">
          <div className="flex items-center gap-2.5">
            <div className="h-8 w-8 rounded-full border-2 border-accent-500 border-t-transparent animate-spin" />
            <div>
              <p className="text-sm font-medium text-[var(--text)]">Waiting for authorization</p>
              <p className="text-xs text-[var(--text-muted)]">Sign in to Cursor in the popup</p>
            </div>
          </div>
          <span className="font-mono text-sm tabular-nums text-[var(--text-muted)]">
            {formatElapsed(elapsed)}
          </span>
        </div>

        <a
          href={verificationUrl}
          target="_blank"
          rel="noopener noreferrer"
          className="block w-full rounded-xl bg-accent-600 px-3 py-2.5 text-center text-sm font-medium text-white shadow-sm transition-colors hover:bg-accent-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/60"
        >
          <span className="inline-flex items-center gap-2">
            <ExternalLink className="h-4 w-4" />
            Open Cursor sign-in
          </span>
        </a>

        <p className="text-center text-xs text-[var(--text-muted)]">
          The link expires in 5 minutes. Complete the sign-in in the other tab.
        </p>

        {elapsed > 240 && (
          <div className="flex items-start gap-2 rounded-lg border border-[color:var(--color-warning)]/30 bg-[color:var(--color-warning)]/10 px-3 py-2">
            <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[color:var(--color-warning)]" />
            <p className="text-xs text-[color:var(--color-warning)]">
              Taking a while? Make sure the popup isn't blocked by your browser.
            </p>
          </div>
        )}
      </div>
    );
  }

  if (status === "starting") {
    return (
      <div className="flex flex-col items-center gap-4 py-6">
        <div className="h-10 w-10 rounded-full border-2 border-accent-500 border-t-transparent animate-spin" />
        <p className="text-sm text-[var(--text-muted)]">Generating secure challenge…</p>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <p className="text-sm text-[var(--text-muted)]">
        Sign in with your Cursor account to route requests through Cursor's API.
        No token to copy — you'll authorize in the browser.
      </p>

      <div className="rounded-xl border border-[var(--border)] bg-[var(--bg-subtle)] p-4">
        <h3 className="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)] mb-3">
          How it works
        </h3>
        <ol className="space-y-2.5">
          <Step num={1} text="We generate a secure PKCE challenge locally" />
          <Step num={2} text="A popup opens for you to sign in to Cursor" />
          <Step num={3} text="Once authorized, your token is encrypted and stored" />
        </ol>
      </div>

      <Button onClick={start} className="w-full">
        <ExternalLink className="h-4 w-4" />
        Sign in with Cursor
      </Button>

      <button
        onClick={onUseImport}
        className="w-full text-center text-xs text-[var(--text-muted)] underline-offset-2 hover:text-[var(--text)] hover:underline"
      >
        Or paste a token from the Cursor IDE
      </button>
    </div>
  );
}

function Step({ num, text }: { num: number; text: string }) {
  return (
    <li className="flex items-start gap-2.5">
      <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-accent-100 text-[10px] font-bold text-accent-700 dark:bg-accent-800/40 dark:text-accent-200">
        {num}
      </span>
      <span className="text-sm text-[var(--text)]">{text}</span>
    </li>
  );
}

// ImportFlow keeps the original paste-token path as a fallback.
function ImportFlow({ onClose, onUseLogin }: { onClose: () => void; onUseLogin: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);

  const submit = async () => {
    if (!token.trim()) {
      setError("Please enter a token from Cursor IDE");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await api.cursorImport(token.trim());
      setDone(true);
      qc.invalidateQueries({ queryKey: ["accounts"] });
      toast.success("Cursor connected", "Token imported successfully.");
      setTimeout(onClose, 1200);
    } catch (e) {
      setError((e as Error).message);
      toast.error("Cursor import failed", (e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  if (done) {
    return <Done provider="Cursor" />;
  }

  return (
    <div className="space-y-4">
      <p className="text-sm text-[var(--text-muted)]">
        Paste the access token from your Cursor IDE. You can find it in
        the Cursor settings under your account section.
      </p>

      <div className="rounded-xl border border-[var(--border)] bg-[var(--bg-subtle)] p-4">
        <h3 className="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)] mb-3">
          How to get the token
        </h3>
        <ol className="space-y-2.5">
          <Step num={1} text="Open Cursor IDE settings" />
          <Step num={2} text="Navigate to your account section" />
          <Step num={3} text="Copy the access token" />
        </ol>
      </div>

      <Field label="Access Token">
        <Input
          value={token}
          onChange={(e) => setToken(e.target.value)}
          placeholder="Paste your Cursor access token…"
          className="font-mono"
        />
      </Field>

      {error && <ErrorBanner message={error} />}

      <Button className="w-full" onClick={submit} disabled={busy || !token.trim()}>
        {busy ? "Importing…" : "Import Token"}
      </Button>

      <button
        onClick={onUseLogin}
        className="w-full text-center text-xs text-[var(--text-muted)] underline-offset-2 hover:text-[var(--text)] hover:underline"
      >
        Or sign in with your Cursor account
      </button>
    </div>
  );
}
