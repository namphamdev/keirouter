import { useEffect, useRef } from "react";
import { useSearchParams, useNavigate } from "react-router-dom";
import { CheckCircle, XCircle } from "lucide-react";

/**
 * OAuthCallback is the landing page after a provider redirects back to the
 * dashboard.  It reads the status from the URL (set by the backend), notifies
 * the opener tab via postMessage, then redirects back to the provider detail
 * page so the user sees the newly-connected account.
 */
export function OAuthCallbackPage() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const status = params.get("status") ?? "error";
  const provider = params.get("provider") ?? "";
  const message = params.get("message") ?? "";
  const ok = status === "success";
  const didNotify = useRef(false);

  useEffect(() => {
    if (didNotify.current) return;
    didNotify.current = true;

    // Notify the opener tab (the connect modal) so it can refresh and close.
    if (window.opener) {
      try {
        window.opener.postMessage(
          { type: "oauth-callback", status, provider, message },
          "*",
        );
      } catch {
        // opener may be gone or cross-origin — ignore
      }
    }

    // After a short delay so the user sees the result, redirect back to the
    // provider detail page (or close if this was a popup).
    const t = setTimeout(() => {
      if (window.opener) {
        // Was opened as a popup — try to close and let the opener handle it.
        try { window.close(); } catch { /* ignore */ }
      }
      // Navigate to the provider detail page (works for both popup and direct).
      if (provider) {
        navigate(`/providers/${provider}`, { replace: true });
      } else {
        navigate("/providers", { replace: true });
      }
    }, 1800);

    return () => clearTimeout(t);
  }, [status, provider, navigate]);

  return (
    <div className="flex min-h-screen items-center justify-center bg-[var(--bg)] p-4">
      <div className="w-full max-w-sm space-y-4 rounded-2xl border border-[var(--border)] bg-[var(--bg-elevated)] p-8 text-center shadow-[var(--shadow-float)]">
        {ok ? (
          <>
            <CheckCircle className="mx-auto h-10 w-10 text-emerald-500" />
            <h1 className="text-sm font-semibold text-[var(--text)]">
              Connected{provider ? ` to ${provider}` : ""}
            </h1>
            <p className="text-xs text-[var(--text-muted)]">
              Redirecting back…
            </p>
          </>
        ) : (
          <>
            <XCircle className="mx-auto h-10 w-10 text-red-500" />
            <h1 className="text-sm font-semibold text-[var(--text)]">
              Connection failed
            </h1>
            <p className="text-xs text-[var(--text-muted)]">
              {message || "An unknown error occurred."}
            </p>
            <p className="text-xs text-[var(--text-muted)]">
              Redirecting back…
            </p>
          </>
        )}
      </div>
    </div>
  );
}
