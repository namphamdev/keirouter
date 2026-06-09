import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Sparkles, Zap, MessageSquare, Layers, Route, Wifi, Monitor, Database, Clock,
  ArrowUpCircle, CheckCircle2, ExternalLink,
  Gauge, Eye, EyeOff, KeyRound, Download, Upload, ShieldCheck, Info,
} from "lucide-react";
import { api, type EndpointSettings } from "../lib/api";
import { PageHeader } from "../components/Layout";
import { useUpdateInfo } from "../components/UpdateNotification";
import { useToast } from "../components/Toast";
import {
  Card, SectionHeader, Spinner, Toggle, SegmentedControl, ErrorBanner, Button, Input, Field,
  SettingsSection, Modal,
} from "../components/ui";

// Caveman compression maps to a Gentle / Balanced / Strong segmented control.
const cavemanOptions = [
  { value: "lite", label: "Gentle" },
  { value: "full", label: "Balanced" },
  { value: "ultra", label: "Strong" },
];
const cavemanHints: Record<string, string> = {
  lite: "Drop filler, keep full sentences.",
  full: "Terse caveman style, fragments OK.",
  ultra: "Maximum compression, telegraphic.",
};

const terseOptions = [
  { value: "light", label: "Gentle" },
  { value: "medium", label: "Balanced" },
  { value: "aggressive", label: "Strong" },
];
const terseHints: Record<string, string> = {
  light: "Trim pleasantries.",
  medium: "Bullets, minimal prose.",
  aggressive: "Bare technical minimum.",
};

const isRoundRobin = (strategy: string) =>
  strategy === "round-robin" || strategy === "round_robin" || strategy === "smart-round-robin" || strategy === "smart_round_robin";

export function SettingsPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const settings = useQuery({ queryKey: ["endpoint-settings"], queryFn: () => api.endpointSettings() });
  const [local, setLocal] = useState<EndpointSettings | null>(null);

  useEffect(() => {
    if (settings.data) setLocal(settings.data);
  }, [settings.data]);

  const save = useMutation({
    mutationFn: (patch: Partial<EndpointSettings>) => api.updateEndpointSettings(patch),
    onSuccess: (data) => {
      setLocal(data);
      qc.setQueryData(["endpoint-settings"], data);
    },
    onError: (e) => toast.error("Settings save failed", (e as Error).message),
  });

  const update = (patch: Partial<EndpointSettings>) => {
    if (local) setLocal({ ...local, ...patch });
    save.mutate(patch);
  };

  return (
    <>
      <PageHeader
        title="Settings"
        icon={Sparkles}
        description="Configure token saving, routing strategy, network, and more."
      />

      {settings.isLoading || !local ? (
        <Spinner />
      ) : (
        <div className="space-y-10">
          {/* ── Token Saving ───────────────────────────────────────── */}
          <SettingsSection title="Token Saving" icon={Zap}>
            <Card>
              <SectionHeader
                title="RTK input compression"
                description="Compresses bulky tool outputs (diffs, greps, listings, build logs) before they reach the model. Saves input tokens. Safe by design — never corrupts content."
                icon={Zap}
              />
              <div className="flex items-center justify-between border-t border-[var(--border)] px-6 py-4">
                <span className="text-sm font-medium">Enable RTK token saver</span>
                <Toggle checked={local.rtk_enabled} onChange={(v) => update({ rtk_enabled: v })} />
              </div>
            </Card>

            <Card>
              <SectionHeader
                title="Caveman output compression"
                description="Instructs the model to answer tersely (caveman style) — keeps all technical substance, drops filler. Cuts output tokens 65-75%."
                icon={MessageSquare}
              />
              <div className="flex items-center justify-between border-t border-[var(--border)] px-6 py-4">
                <span className="text-sm font-medium">Enable caveman mode</span>
                <Toggle checked={local.caveman_enabled} onChange={(v) => update({ caveman_enabled: v, ...(v ? { terse_enabled: false } : {}) })} />
              </div>
              {local.caveman_enabled && (
                <div className="flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border)] px-6 py-4">
                  <div>
                    <p className="text-sm font-medium">Compression level</p>
                    <p className="mt-0.5 text-xs text-[var(--text-muted)]">{cavemanHints[local.caveman_level]}</p>
                  </div>
                  <SegmentedControl
                    value={local.caveman_level}
                    onChange={(v) => update({ caveman_level: v })}
                    options={cavemanOptions}
                  />
                </div>
              )}
            </Card>

            <Card>
              <SectionHeader
                title="Terse mode (alternative)"
                description="KeiRouter's own concise-output directive. An alternative to caveman; both inject a system instruction, so pick one."
                icon={Layers}
                iconTone="neutral"
              />
              <div className="flex items-center justify-between border-t border-[var(--border)] px-6 py-4">
                <span className="text-sm font-medium">Enable terse mode</span>
                <Toggle checked={local.terse_enabled} onChange={(v) => update({ terse_enabled: v, ...(v ? { caveman_enabled: false } : {}) })} />
              </div>
              {local.terse_enabled && (
                <div className="flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border)] px-6 py-4">
                  <div>
                    <p className="text-sm font-medium">Terse level</p>
                    <p className="mt-0.5 text-xs text-[var(--text-muted)]">{terseHints[local.terse_level]}</p>
                  </div>
                  <SegmentedControl
                    value={local.terse_level}
                    onChange={(v) => update({ terse_level: v })}
                    options={terseOptions}
                  />
                </div>
              )}
            </Card>
          </SettingsSection>

          {/* ── Routing ────────────────────────────────────────────── */}
          <SettingsSection title="Routing" icon={Route}>
            <RoutingStrategy local={local} update={update} />
          </SettingsSection>

          {/* ── Network & Timeouts ─────────────────────────────────── */}
          <SettingsSection title="Network & Timeouts" icon={Gauge}>
            <TimeoutSettings local={local} update={update} />
            <NetworkSettings local={local} update={update} />
          </SettingsSection>

          {/* ── Observability ──────────────────────────────────────── */}
          <SettingsSection title="Observability" icon={Monitor}>
            <Card>
              <SectionHeader
                title="Request detail recording"
                description="Record request details for inspection in the logs view."
                icon={Monitor}
              />
              <div className="flex items-center justify-between border-t border-[var(--border)] px-6 py-4">
                <span className="text-sm font-medium">Enable request detail recording</span>
                <Toggle
                  checked={local.observability_enabled !== false}
                  onChange={(v) => update({ observability_enabled: v })}
                />
              </div>
            </Card>
          </SettingsSection>

          {/* ── Data & Updates ─────────────────────────────────────── */}
          <SettingsSection title="Data & Updates" icon={Database}>
            <DatabaseSettings />
            <UpdatesSettings />
          </SettingsSection>

          {save.isError && <ErrorBanner message={`Failed to save: ${(save.error as Error)?.message ?? "unknown error"}`} />}
        </div>
      )}
    </>
  );
}

function RoutingStrategy({
  local,
  update,
}: {
  local: EndpointSettings;
  update: (patch: Partial<EndpointSettings>) => void;
}) {
  return (
    <Card>
      <SectionHeader
        title="Routing Strategy"
        description="Control how requests are distributed across accounts and chains."
        icon={Route}
      />
      <div className="divide-y divide-[var(--border)] border-t border-[var(--border)]">
        <div className="flex items-center justify-between gap-4 px-6 py-4">
          <div>
            <p className="text-sm font-medium">Provider group round robin</p>
            <p className="mt-0.5 text-xs text-[var(--text-muted)]">Cycle through accounts in the same provider/model group</p>
          </div>
          <Toggle
            checked={isRoundRobin(local.routing_strategy)}
            onChange={() => update({ routing_strategy: isRoundRobin(local.routing_strategy) ? "fill-first" : "round-robin" })}
          />
        </div>

        {isRoundRobin(local.routing_strategy) && (
          <div className="flex items-center justify-between gap-4 px-6 py-4">
            <div>
              <p className="text-sm font-medium">Provider sticky limit</p>
              <p className="mt-0.5 text-xs text-[var(--text-muted)]">Calls per account before switching</p>
            </div>
            <Input
              type="number"
              min={1}
              max={10}
              value={local.sticky_limit || 3}
              onChange={(e) => update({ sticky_limit: parseInt(e.target.value) || 3 })}
              className="w-20 text-center"
            />
          </div>
        )}

        <div className="flex items-center justify-between gap-4 px-6 py-4">
          <div>
            <p className="text-sm font-medium">Chain round robin</p>
            <p className="mt-0.5 text-xs text-[var(--text-muted)]">Cycle through providers in chains instead of always starting with first</p>
          </div>
          <Toggle
            checked={isRoundRobin(local.combo_strategy)}
            onChange={() => update({ combo_strategy: isRoundRobin(local.combo_strategy) ? "fallback" : "round-robin" })}
          />
        </div>

        {isRoundRobin(local.combo_strategy) && (
          <div className="flex items-center justify-between gap-4 px-6 py-4">
            <div>
              <p className="text-sm font-medium">Chain sticky limit</p>
              <p className="mt-0.5 text-xs text-[var(--text-muted)]">Calls per chain model before switching</p>
            </div>
            <Input
              type="number"
              min={1}
              max={100}
              value={local.combo_sticky_limit || 1}
              onChange={(e) => update({ combo_sticky_limit: parseInt(e.target.value) || 1 })}
              className="w-20 text-center"
            />
          </div>
        )}

        <div className="px-6 py-3">
          <p className="text-xs text-[var(--text-muted)] italic">
            {isRoundRobin(local.routing_strategy)
              ? `Distributing requests across all available accounts with ${local.sticky_limit || 3} calls per account.`
              : "Using accounts in priority order (Fill First)."}
            {isRoundRobin(local.combo_strategy)
              ? ` Chains rotate after ${local.combo_sticky_limit || 1} call${(local.combo_sticky_limit || 1) === 1 ? "" : "s"} per model.`
              : " Chains always start with their first model."}
          </p>
        </div>
      </div>
    </Card>
  );
}

function TimeoutSettings({
  local,
  update,
}: {
  local: EndpointSettings;
  update: (patch: Partial<EndpointSettings>) => void;
}) {
  return (
    <Card>
      <SectionHeader
        title="Timeouts"
        description="Fine-tune upstream connection and streaming timeouts. Increase for slow providers or reasoning models (Deepseek, GLM) that think before streaming."
        icon={Clock}
      />
      <div className="divide-y divide-[var(--border)] border-t border-[var(--border)]">
        <div className="px-6 py-4">
          <Field label="Connect timeout (seconds)">
            <Input
              type="number"
              min={5}
              max={300}
              value={Math.round((local.response_header_timeout_ms || 60000) / 1000)}
              onChange={(e) => {
                const sec = parseInt(e.target.value) || 60;
                update({ response_header_timeout_ms: sec * 1000 });
              }}
              placeholder="60"
              className="w-24 text-center"
            />
            <p className="mt-1 text-xs text-[var(--text-muted)]">
              Max time waiting for upstream to send response headers. Default: 60s. Increase for slow providers.
            </p>
          </Field>
        </div>
        <div className="px-6 py-4">
          <Field label="Stream stall timeout (seconds)">
            <Input
              type="number"
              min={10}
              max={600}
              value={Math.round((local.stream_stall_timeout_ms || 120000) / 1000)}
              onChange={(e) => {
                const sec = parseInt(e.target.value) || 120;
                update({ stream_stall_timeout_ms: sec * 1000 });
              }}
              placeholder="120"
              className="w-24 text-center"
            />
            <p className="mt-1 text-xs text-[var(--text-muted)]">
              Abort stream if no data received for this long. Default: 120s. Increase for reasoning models that think before streaming.
            </p>
          </Field>
        </div>
        <div className="px-6 py-4">
          <Field label="Request timeout (seconds)">
            <Input
              type="number"
              min={30}
              max={3600}
              value={Math.round((local.request_timeout_ms || 300000) / 1000)}
              onChange={(e) => {
                const sec = parseInt(e.target.value) || 300;
                update({ request_timeout_ms: sec * 1000 });
              }}
              placeholder="300"
              className="w-24 text-center"
            />
            <p className="mt-1 text-xs text-[var(--text-muted)]">
              Upper bound for non-streaming requests. Default: 300s (5 min).
            </p>
          </Field>
        </div>
      </div>
    </Card>
  );
}

function NetworkSettings({
  local,
  update,
}: {
  local: EndpointSettings;
  update: (patch: Partial<EndpointSettings>) => void;
}) {
  const [testResult, setTestResult] = useState<string>("");
  const [testing, setTesting] = useState(false);

  const testProxy = async () => {
    if (!local.outbound_proxy_url) return;
    setTesting(true);
    setTestResult("");
    try {
      const res = await api.testProxy(local.outbound_proxy_url);
      if (res.ok) {
        setTestResult(`Proxy test OK (${res.status}) in ${res.elapsedMs}ms`);
      } else {
        setTestResult(`Proxy test failed: ${res.error}`);
      }
    } catch (e) {
      setTestResult(`Error: ${(e as Error).message}`);
    } finally {
      setTesting(false);
    }
  };

  return (
    <Card>
      <SectionHeader
        title="Network"
        description="Configure outbound proxy for provider requests."
        icon={Wifi}
      />
      <div className="divide-y divide-[var(--border)] border-t border-[var(--border)]">
        <div className="flex items-center justify-between gap-4 px-6 py-4">
          <div>
            <p className="text-sm font-medium">Outbound Proxy</p>
            <p className="mt-0.5 text-xs text-[var(--text-muted)]">Enable proxy for OAuth + provider outbound requests.</p>
          </div>
          <Toggle
            checked={local.outbound_proxy_enabled}
            onChange={(v) => update({ outbound_proxy_enabled: v })}
          />
        </div>

        {local.outbound_proxy_enabled && (
          <>
            <div className="px-6 py-4">
              <Field label="Proxy URL">
                <Input
                  placeholder="http://127.0.0.1:7897"
                  value={local.outbound_proxy_url}
                  onChange={(e) => update({ outbound_proxy_url: e.target.value })}
                />
                <p className="mt-1 text-xs text-[var(--text-muted)]">Leave empty to inherit existing env proxy (if any).</p>
              </Field>
            </div>
            <div className="px-6 py-4">
              <Field label="No Proxy">
                <Input
                  placeholder="localhost,127.0.0.1"
                  value={local.outbound_no_proxy}
                  onChange={(e) => update({ outbound_no_proxy: e.target.value })}
                />
                <p className="mt-1 text-xs text-[var(--text-muted)]">Comma-separated hostnames/domains to bypass the proxy.</p>
              </Field>
            </div>
            <div className="flex items-center gap-3 px-6 py-4">
              <Button variant="ghost" onClick={testProxy} disabled={testing || !local.outbound_proxy_url}>
                {testing ? "Testing…" : "Test proxy URL"}
              </Button>
              {testResult && (
                <span className={`text-xs ${testResult.startsWith("Proxy test OK") ? "text-green-600 dark:text-green-400" : "text-[color:var(--color-danger)]"}`}>
                  {testResult}
                </span>
              )}
            </div>
          </>
        )}
      </div>
    </Card>
  );
}

function PassphraseInput({
  id,
  value,
  onChange,
  show,
  onToggleShow,
  placeholder,
  autoFocus,
  ariaInvalid,
}: {
  id: string;
  value: string;
  onChange: (v: string) => void;
  show: boolean;
  onToggleShow: () => void;
  placeholder?: string;
  autoFocus?: boolean;
  ariaInvalid?: boolean;
}) {
  return (
    <div className="relative">
      <Input
        id={id}
        type={show ? "text" : "password"}
        value={value}
        autoFocus={autoFocus}
        autoComplete="new-password"
        spellCheck={false}
        placeholder={placeholder}
        aria-invalid={ariaInvalid}
        onChange={(e) => onChange(e.target.value)}
        className="pr-11"
      />
      <button
        type="button"
        onClick={onToggleShow}
        aria-label={show ? "Hide passphrase" : "Show passphrase"}
        className="absolute inset-y-0 right-0 flex w-10 items-center justify-center text-[var(--text-muted)] transition-colors hover:text-[var(--text)] focus:outline-none focus-visible:text-[var(--text)]"
      >
        {show ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
      </button>
    </div>
  );
}

function strengthOf(pass: string): { label: string; tone: "muted" | "weak" | "ok" | "strong"; pct: number } {
  if (!pass) return { label: "No passphrase — local backup", tone: "muted", pct: 0 };
  let score = 0;
  if (pass.length >= 8) score++;
  if (pass.length >= 14) score++;
  if (/[A-Z]/.test(pass) && /[a-z]/.test(pass)) score++;
  if (/\d/.test(pass)) score++;
  if (/[^A-Za-z0-9]/.test(pass)) score++;
  if (score <= 2) return { label: "Weak — short or simple", tone: "weak", pct: 33 };
  if (score <= 3) return { label: "OK — could be stronger", tone: "ok", pct: 66 };
  return { label: "Strong", tone: "strong", pct: 100 };
}

function DatabaseSettings() {
  const toast = useToast();
  const importRef = useRef<HTMLInputElement>(null);
  const [loading, setLoading] = useState(false);

  // Export modal state
  const [exportOpen, setExportOpen] = useState(false);
  const [usePortable, setUsePortable] = useState(false);
  const [exportPass, setExportPass] = useState("");
  const [exportConfirm, setExportConfirm] = useState("");
  const [showExportPass, setShowExportPass] = useState(false);

  // Import modal state
  const [importOpen, setImportOpen] = useState(false);
  const [pendingPayload, setPendingPayload] = useState<Record<string, unknown> | null>(null);
  const [importPass, setImportPass] = useState("");
  const [showImportPass, setShowImportPass] = useState(false);
  const [importError, setImportError] = useState<string | null>(null);

  const resetExport = () => {
    setExportOpen(false);
    setUsePortable(false);
    setExportPass("");
    setExportConfirm("");
    setShowExportPass(false);
  };

  const resetImport = () => {
    setImportOpen(false);
    setPendingPayload(null);
    setImportPass("");
    setShowImportPass(false);
    setImportError(null);
    if (importRef.current) importRef.current.value = "";
  };

  const downloadBackup = async (pass: string | undefined) => {
    setLoading(true);
    try {
      const data = await api.exportDatabase(pass || undefined);
      const content = JSON.stringify(data, null, 2);
      const blob = new Blob([content], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      const stamp = new Date().toISOString().replace(/[.:]/g, "-");
      a.href = url;
      a.download = `keirouter-backup${pass ? "-portable" : ""}-${stamp}.json`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
      toast.success(
        "Backup downloaded",
        pass
          ? "Portable backup saved. Keep the passphrase safe — it is required to import on another machine."
          : "Local backup saved. It only restores on this machine's master key.",
      );
      resetExport();
    } catch (e) {
      toast.error("Export failed", (e as Error).message);
    } finally {
      setLoading(false);
    }
  };

  const handleExportSubmit = () => {
    if (usePortable) {
      const p = exportPass.trim();
      if (!p) return;
      if (p !== exportConfirm.trim()) return;
      void downloadBackup(p);
    } else {
      void downloadBackup(undefined);
    }
  };

  const handleFilePicked = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;

    try {
      const raw = await file.text();
      const payload = JSON.parse(raw);

      if (payload && (payload as { portable?: boolean }).portable === true) {
        setPendingPayload(payload);
        setImportPass("");
        setShowImportPass(false);
        setImportError(null);
        setImportOpen(true);
        return;
      }

      setLoading(true);
      const result = await api.importDatabase(payload);
      toast.success("Import complete", `${result.imported} records restored. Existing data was merged or updated.`);
      if (importRef.current) importRef.current.value = "";
    } catch (e) {
      toast.error("Import failed", (e as Error).message);
      if (importRef.current) importRef.current.value = "";
    } finally {
      setLoading(false);
    }
  };

  const submitImportWithPass = async () => {
    const p = importPass.trim();
    if (!p) {
      setImportError("Passphrase required for portable backups.");
      return;
    }
    if (!pendingPayload) {
      setImportError("No backup loaded. Select a file first.");
      return;
    }
    setImportError(null);
    setLoading(true);
    try {
      const result = await api.importDatabase(pendingPayload, p);
      toast.success("Import complete", `${result.imported} records restored. Existing data was merged or updated.`);
      resetImport();
    } catch (e) {
      setImportError((e as Error).message || "Import failed. Wrong passphrase?");
    } finally {
      setLoading(false);
    }
  };

  const exportStrength = strengthOf(exportPass);
  const exportMismatch =
    usePortable && exportConfirm.length > 0 && exportPass !== exportConfirm;
  const exportDisabled =
    loading ||
    (usePortable && (!exportPass.trim() || exportPass !== exportConfirm));

  return (
    <>
      <Card>
        <SectionHeader
          title="Database"
          description="Export or import your KeiRouter configuration."
          icon={Database}
        />
        <div className="flex flex-wrap items-center gap-3 border-t border-[var(--border)] px-6 py-4">
          <Button variant="ghost" onClick={() => setExportOpen(true)} disabled={loading}>
            <Download className="h-4 w-4" />
            Download Backup
          </Button>
          <Button variant="ghost" onClick={() => importRef.current?.click()} disabled={loading}>
            <Upload className="h-4 w-4" />
            Import Backup
          </Button>
          <input
            ref={importRef}
            type="file"
            accept="application/json,.json"
            className="hidden"
            onChange={handleFilePicked}
          />
        </div>
      </Card>

      <Modal
        open={exportOpen}
        onClose={() => (loading ? null : resetExport())}
        title="Download backup"
        subtitle="Choose how credentials are encrypted in the export file."
        maxWidth="max-w-md"
      >
        <div className="max-h-[55vh] space-y-5 overflow-y-auto px-6 py-5">
          <div className="space-y-2">
            <label className="flex cursor-pointer items-start gap-3 rounded-xl border border-[var(--border)] px-4 py-3 transition-colors hover:bg-ink-50 dark:hover:bg-ink-900/40">
              <input
                type="radio"
                name="export-mode"
                checked={!usePortable}
                onChange={() => setUsePortable(false)}
                className="mt-1 h-4 w-4 accent-secondary-600"
              />
              <div className="flex-1">
                <div className="flex items-center gap-2 text-sm font-medium text-[var(--text)]">
                  <ShieldCheck className="h-4 w-4 text-accent-600 dark:text-accent-300" />
                  Local backup
                </div>
                <p className="mt-0.5 text-xs leading-relaxed text-[var(--text-muted)]">
                  Tied to this machine's master key. Only restores on this install. No passphrase needed.
                </p>
              </div>
            </label>

            <label className="flex cursor-pointer items-start gap-3 rounded-xl border border-[var(--border)] px-4 py-3 transition-colors hover:bg-ink-50 dark:hover:bg-ink-900/40">
              <input
                type="radio"
                name="export-mode"
                checked={usePortable}
                onChange={() => setUsePortable(true)}
                className="mt-1 h-4 w-4 accent-secondary-600"
              />
              <div className="flex-1">
                <div className="flex items-center gap-2 text-sm font-medium text-[var(--text)]">
                  <KeyRound className="h-4 w-4 text-secondary-600 dark:text-secondary-300" />
                  Portable backup
                </div>
                <p className="mt-0.5 text-xs leading-relaxed text-[var(--text-muted)]">
                  Re-keys credentials to a passphrase so you can restore on another machine.
                </p>
              </div>
            </label>
          </div>

          {usePortable && (
            <div className="space-y-3 rounded-xl border border-[var(--border)] bg-[var(--bg)] p-4">
              <Field label="Passphrase">
                <PassphraseInput
                  id="export-pass"
                  value={exportPass}
                  onChange={setExportPass}
                  show={showExportPass}
                  onToggleShow={() => setShowExportPass((s) => !s)}
                  placeholder="At least 8 characters"
                  autoFocus
                />
                <div className="mt-2">
                  <div className="h-1.5 w-full overflow-hidden rounded-full bg-ink-200 dark:bg-ink-800">
                    <div
                      className={`h-full transition-all duration-300 ${
                        exportStrength.tone === "strong"
                          ? "bg-accent-500"
                          : exportStrength.tone === "ok"
                            ? "bg-amber-500"
                            : exportStrength.tone === "weak"
                              ? "bg-[color:var(--color-danger)]"
                              : "bg-transparent"
                      }`}
                      style={{ width: `${exportStrength.pct}%` }}
                    />
                  </div>
                  <p className="mt-1 text-[11px] font-medium text-[var(--text-muted)]">
                    {exportStrength.label}
                  </p>
                </div>
              </Field>

              <Field label="Confirm passphrase">
                <PassphraseInput
                  id="export-confirm"
                  value={exportConfirm}
                  onChange={setExportConfirm}
                  show={showExportPass}
                  onToggleShow={() => setShowExportPass((s) => !s)}
                  placeholder="Re-enter passphrase"
                  ariaInvalid={exportMismatch}
                />
                {exportMismatch && (
                  <p className="mt-1 text-xs text-[color:var(--color-danger)]">
                    Passphrases do not match.
                  </p>
                )}
              </Field>

              <div className="flex items-start gap-2 rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:bg-amber-900/20 dark:text-amber-200">
                <Info className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                <span>Store this passphrase safely. Without it the backup cannot be restored — there is no recovery.</span>
              </div>
            </div>
          )}
        </div>

        <div className="flex items-center justify-end gap-2 border-t border-[var(--border)] px-6 py-4">
          <Button variant="ghost" onClick={resetExport} disabled={loading}>
            Cancel
          </Button>
          <Button variant="primary" onClick={handleExportSubmit} disabled={exportDisabled}>
            <Download className="h-4 w-4" />
            {loading ? "Preparing…" : "Download"}
          </Button>
        </div>
      </Modal>

      <Modal
        open={importOpen}
        onClose={() => (loading ? null : resetImport())}
        title="Portable backup detected"
        subtitle="Enter the passphrase that was used when this backup was exported."
      >
        <div className="space-y-4 px-6 py-5">
          <Field label="Passphrase">
            <PassphraseInput
              id="import-pass"
              value={importPass}
              onChange={(v) => {
                setImportPass(v);
                if (importError) setImportError(null);
              }}
              show={showImportPass}
              onToggleShow={() => setShowImportPass((s) => !s)}
              placeholder="Passphrase from export"
              autoFocus
              ariaInvalid={!!importError}
            />
          </Field>

          {importError && <ErrorBanner message={importError} />}

          <div className="flex items-start gap-2 rounded-lg bg-ink-100 px-3 py-2 text-xs text-[var(--text-muted)] dark:bg-ink-800/60">
            <Info className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            <span>Existing data is merged or updated. The import will not delete records that aren't in the backup.</span>
          </div>
        </div>

        <div className="flex items-center justify-end gap-2 border-t border-[var(--border)] px-6 py-4">
          <Button variant="ghost" onClick={resetImport} disabled={loading}>
            Cancel
          </Button>
          <Button
            variant="primary"
            onClick={submitImportWithPass}
            disabled={loading || !importPass.trim()}
          >
            <Upload className="h-4 w-4" />
            {loading ? "Importing…" : "Restore"}
          </Button>
        </div>
      </Modal>
    </>
  );
}

function UpdatesSettings() {
  const { data, isLoading, isError } = useUpdateInfo();

  const publishedLabel = data?.published_at
    ? new Date(data.published_at).toLocaleDateString(undefined, {
        year: "numeric",
        month: "short",
        day: "numeric",
      })
    : "";

  return (
    <Card>
      <SectionHeader
        title="Updates"
        description="Check for new KeiRouter releases and read the latest changelog."
        icon={ArrowUpCircle}
      />
      <div className="border-t border-[var(--border)] px-6 py-4">
        {isLoading ? (
          <Spinner />
        ) : isError || !data || !data.checked ? (
          <p className="text-sm text-[var(--text-muted)]">
            Could not reach GitHub to check for updates. Current version:{" "}
            <span className="font-mono text-[var(--text)]">{data?.current ?? "dev"}</span>
          </p>
        ) : (
          <div className="space-y-4">
            <div className="flex flex-wrap items-center gap-x-6 gap-y-2">
              <div>
                <p className="text-[10px] font-semibold uppercase tracking-wider text-[var(--text-muted)]">
                  Current
                </p>
                <p className="font-mono text-sm">{data.current}</p>
              </div>
              <div>
                <p className="text-[10px] font-semibold uppercase tracking-wider text-[var(--text-muted)]">
                  Latest
                </p>
                <p className="font-mono text-sm">{data.latest || "—"}</p>
              </div>
              <div className="ml-auto">
                {data.update_available ? (
                  <span className="inline-flex items-center gap-1.5 rounded-full bg-accent-100 px-3 py-1 text-xs font-medium text-accent-700 dark:bg-accent-900/40 dark:text-accent-200">
                    <ArrowUpCircle className="h-3.5 w-3.5" />
                    Update available
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-1.5 rounded-full bg-green-100 px-3 py-1 text-xs font-medium text-green-700 dark:bg-green-900/30 dark:text-green-300">
                    <CheckCircle2 className="h-3.5 w-3.5" />
                    Up to date
                  </span>
                )}
              </div>
            </div>

            {data.update_available && data.changelog && (
              <div>
                <div className="mb-1 flex items-center justify-between">
                  <p className="text-[10px] font-semibold uppercase tracking-wider text-[var(--text-muted)]">
                    Changelog{publishedLabel ? ` · ${publishedLabel}` : ""}
                  </p>
                  {data.html_url && (
                    <a
                      href={data.html_url}
                      target="_blank"
                      rel="noreferrer noopener"
                      className="flex items-center gap-1 text-xs text-accent-600 hover:underline dark:text-accent-300"
                    >
                      View on GitHub
                      <ExternalLink className="h-3 w-3" />
                    </a>
                  )}
                </div>
                <div className="max-h-80 overflow-y-auto rounded-lg border border-[var(--border)] bg-[var(--bg)] p-4">
                  <pre className="whitespace-pre-wrap break-words font-sans text-sm leading-relaxed text-[var(--text)]">
                    {data.changelog}
                  </pre>
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </Card>
  );
}
