import { useRef, useState, useCallback } from "react";
import { toPng } from "html-to-image";
import { Download, Check } from "lucide-react";
import type { UsageInsights } from "../lib/api";

// ─── Helpers ─────────────────────────────────────────────────────────────────

function fmtNum(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return n.toLocaleString();
}

const periodLabels: Record<string, string> = {
  today: "Today",
  "24h": "Last 24 Hours",
  week: "Last 7 Days",
  month: "Last 30 Days",
};

// Premium Minimalist Dark Palette (Anti-Slop)
const C = {
  bg: "#0A0A0B",
  surface: "#121214",
  border: "#27272A",
  borderLight: "#18181B",
  text: "#FAFAFA",
  textSecondary: "#A1A1AA",
  textMuted: "#71717A",
  accent: "#E4E4E7", // Sharp white for main numbers
  accentSecondary: "#38BDF8", // Refined sharp blue for secondary tone
  positive: "#10B981", // Emerald for reductions
};

// ─── Hidden Card (rendered off-screen for capture) ───────────────────────────

interface SavingsCardData {
  costSaved: number;
  tokensSaved: number;
  savingsPct: number;
  totalRequests: number;
  period: string;
  rtkActive: boolean;
  cavemanActive: boolean;
  terseActive: boolean;
  actualCost: number;
}

function SavingsCardContent({ data }: { data: SavingsCardData }) {
  const {
    costSaved,
    tokensSaved,
    savingsPct,
    totalRequests,
    period,
    rtkActive,
    cavemanActive,
    terseActive,
    actualCost,
  } = data;

  const optimizers = [
    rtkActive && { name: "RTK", desc: "Tokenizer" },
    cavemanActive && { name: "Caveman", desc: "Compression" },
    terseActive && { name: "Terse", desc: "Compression" },
  ].filter(Boolean) as { name: string; desc: string }[];

  return (
    <div
      style={{
        width: 1200,
        height: 630,
        position: "relative",
        boxSizing: "border-box",
        fontFamily:
          "'-apple-system', 'BlinkMacSystemFont', 'SF Pro Display', 'Inter', sans-serif",
        background: C.bg,
        color: C.text,
        padding: "40px",
      }}
    >
      {/* Outer Border Frame */}
      <div
        style={{
          width: "100%",
          height: "100%",
          border: `1px solid ${C.border}`,
          borderRadius: 12,
          display: "flex",
          flexDirection: "column",
          overflow: "hidden",
        }}
      >
        {/* Header Bar */}
        <div
          style={{
            height: 80,
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            padding: "0 40px",
            borderBottom: `1px solid ${C.border}`,
            background: C.surface,
          }}
        >
          <div style={{ display: "flex", alignItems: "center", gap: 16 }}>
            <img
              src="/keirouter-logo.png"
              alt="KeiRouter"
              style={{ width: 28, height: 28, objectFit: "contain" }}
              crossOrigin="anonymous"
            />
            <span
              style={{
                fontSize: 20,
                fontWeight: 600,
                letterSpacing: "-0.01em",
              }}
            >
              KeiRouter
            </span>
            <div style={{ width: 1, height: 16, background: C.border, margin: "0 8px" }} />
            <span
              style={{
                fontSize: 13,
                fontWeight: 500,
                color: C.textSecondary,
                letterSpacing: "0.1em",
                textTransform: "uppercase",
              }}
            >
              Savings Report
            </span>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <span style={{ fontSize: 14, fontWeight: 500, color: C.textMuted }}>Period</span>
            <span style={{ fontSize: 14, fontWeight: 600, color: C.text }}>{periodLabels[period] || period}</span>
          </div>
        </div>

        {/* Main Split Content */}
        <div style={{ flex: 1, display: "flex" }}>
          
          {/* Left Panel: Primary Metric */}
          <div
            style={{
              flex: "0 0 55%",
              padding: "60px 40px",
              display: "flex",
              flexDirection: "column",
              justifyContent: "center",
              borderRight: `1px solid ${C.border}`,
            }}
          >
            <span
              style={{
                fontSize: 16,
                fontWeight: 500,
                color: C.textSecondary,
                marginBottom: 24,
                letterSpacing: "0.02em",
              }}
            >
              Total Cost Saved
            </span>
            <span
              style={{
                fontSize: 140,
                fontWeight: 700,
                color: C.accent,
                lineHeight: 1,
                letterSpacing: "-0.04em",
                marginBottom: 24,
              }}
            >
              ${costSaved.toFixed(2)}
            </span>
            
            <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
              <div style={{
                background: "rgba(16, 185, 129, 0.1)",
                color: C.positive,
                padding: "8px 16px",
                borderRadius: 6,
                fontSize: 16,
                fontWeight: 600,
                display: "flex",
                alignItems: "center",
                gap: 8,
              }}
              >
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                  <line x1="12" y1="5" x2="12" y2="19"></line>
                  <polyline points="19 12 12 19 5 12"></polyline>
                </svg>
                {savingsPct.toFixed(1)}% Cost Reduction
              </div>
            </div>
          </div>

          {/* Right Panel: Data Grid */}
          <div style={{ flex: 1, display: "flex", flexDirection: "column" }}>
            <div style={{ flex: 1, display: "flex", flexDirection: "column" }}>
              
              {/* Stat Row 1 */}
              <div style={{ flex: 1, display: "flex", borderBottom: `1px solid ${C.border}` }}>
                <StatBox label="Tokens Saved" value={fmtNum(tokensSaved)} borderRight />
                <StatBox label="Total Requests" value={fmtNum(totalRequests)} />
              </div>

              {/* Stat Row 2 */}
              <div style={{ flex: 1, display: "flex", borderBottom: `1px solid ${C.border}` }}>
                <StatBox label="Cost With KeiRouter" value={`$${actualCost.toFixed(2)}`} borderRight />
                <StatBox label="Original Cost (Est.)" value={`$${(actualCost + costSaved).toFixed(2)}`} muted />
              </div>

              {/* Optimizers Row */}
              <div style={{ padding: "32px 40px", flex: 1, display: "flex", flexDirection: "column", justifyContent: "center", background: C.surface }}>
                <span
                  style={{
                    fontSize: 12,
                    fontWeight: 600,
                    color: C.textMuted,
                    textTransform: "uppercase",
                    letterSpacing: "0.1em",
                    marginBottom: 16,
                  }}
                >
                  Active Optimizers
                </span>
                <div style={{ display: "flex", gap: 12 }}>
                  {optimizers.length > 0 ? optimizers.map((opt) => (
                    <div
                      key={opt.name}
                      style={{
                        padding: "6px 12px",
                        border: `1px solid ${C.border}`,
                        borderRadius: 6,
                        background: C.bg,
                        display: "flex",
                        alignItems: "center",
                        gap: 8,
                      }}
                    >
                      <div style={{ width: 6, height: 6, borderRadius: "50%", background: C.accentSecondary }} />
                      <span style={{ fontSize: 14, fontWeight: 500, color: C.text }}>{opt.name}</span>
                    </div>
                  )) : (
                    <span style={{ fontSize: 14, color: C.textMuted }}>None</span>
                  )}
                </div>
              </div>

            </div>
          </div>

        </div>

        {/* Footer Bar */}
        <div
          style={{
            height: 60,
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            padding: "0 40px",
            borderTop: `1px solid ${C.border}`,
            background: C.surface,
          }}
        >
          <span style={{ fontSize: 13, fontWeight: 500, color: C.textSecondary, letterSpacing: "0.02em" }}>
            keirouter.dev
          </span>
          <span style={{ fontSize: 13, fontWeight: 500, color: C.textSecondary, letterSpacing: "0.02em" }}>
            AI Routing, Optimized.
          </span>
        </div>

      </div>
    </div>
  );
}

function StatBox({ label, value, muted, borderRight }: { label: string; value: string, muted?: boolean, borderRight?: boolean }) {
  return (
    <div style={{ 
      flex: 1, 
      padding: "32px 40px", 
      display: "flex", 
      flexDirection: "column", 
      justifyContent: "center",
      borderRight: borderRight ? `1px solid ${C.border}` : "none"
    }}>
      <span
        style={{
          fontSize: 13,
          fontWeight: 500,
          color: C.textMuted,
          marginBottom: 12,
        }}
      >
        {label}
      </span>
      <span
        style={{
          fontSize: 36,
          fontWeight: 600,
          color: muted ? C.textSecondary : C.text,
          letterSpacing: "-0.02em",
        }}
      >
        {value}
      </span>
    </div>
  );
}

// ─── Share Button ────────────────────────────────────────────────────────────

export function SavingsCardShareButton({
  insights,
  period,
}: {
  insights: UsageInsights;
  period: string;
}) {
  const cardRef = useRef<HTMLDivElement>(null);
  const [generating, setGenerating] = useState(false);
  const [done, setDone] = useState(false);

  const { summary, savings } = insights;

  const avgRate = 3;
  const tokensSaved = savings?.slim_tokens_saved ?? 0;
  const costSaved = (tokensSaved / 1_000_000) * avgRate;
  const actualCost = summary.cost_usd;
  const originalCost = actualCost + costSaved;
  const savingsPct = originalCost > 0 ? (costSaved / originalCost) * 100 : 0;

  const cardData: SavingsCardData = {
    costSaved,
    tokensSaved,
    savingsPct,
    totalRequests: summary.total_requests,
    period,
    rtkActive: (savings?.slim_tokens_saved ?? 0) > 0,
    cavemanActive: (savings?.caveman_requests ?? 0) > 0,
    terseActive: (savings?.terse_requests ?? 0) > 0,
    actualCost,
  };

  const handleShare = useCallback(async () => {
    if (!cardRef.current || generating) return;
    setGenerating(true);
    setDone(false);

    try {
      const dataUrl = await toPng(cardRef.current, {
        width: 1200,
        height: 630,
        pixelRatio: 2,
        cacheBust: true,
      });

      const link = document.createElement("a");
      link.download = `keirouter-savings-${period}.png`;
      link.href = dataUrl;
      link.click();
      setDone(true);
      setTimeout(() => setDone(false), 2000);
    } catch (err) {
      console.error("Failed to generate savings card:", err);
    } finally {
      setGenerating(false);
    }
  }, [generating, period]);

  return (
    <>
      {/* Hidden card for image capture */}
      <div
        style={{
          position: "fixed",
          left: "-9999px",
          top: 0,
          zIndex: -1,
          pointerEvents: "none",
        }}
      >
        <div ref={cardRef}>
          <SavingsCardContent data={cardData} />
        </div>
      </div>

      {/* Download button */}
      <button
        onClick={handleShare}
        disabled={generating || summary.total_requests === 0}
        className="inline-flex h-8 items-center gap-1.5 whitespace-nowrap rounded-lg bg-accent-600 px-3 text-xs font-medium text-white shadow-sm transition-all hover:bg-accent-700 active:scale-[0.97] disabled:cursor-not-allowed disabled:opacity-50 dark:bg-accent-500 dark:hover:bg-accent-400"
      >
        {done ? (
          <>
            <Check className="h-3.5 w-3.5" />
            <span className="hidden sm:inline">Downloaded!</span>
          </>
        ) : generating ? (
          <>
            <div className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-white/30 border-t-white" />
            <span className="hidden sm:inline">Generating…</span>
          </>
        ) : (
          <>
            <img
              src="/keirouter-logo.png"
              alt=""
              className="h-3.5 w-3.5 object-contain"
            />
            <span className="hidden sm:inline">Savings Card</span>
            <Download className="h-3.5 w-3.5 opacity-70" />
          </>
        )}
      </button>
    </>
  );
}