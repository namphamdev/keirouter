import { useState, useEffect, useRef, useMemo, useCallback } from "react";
import { createPortal } from "react-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Layers, Plus, Trash2, X, ArrowRight, Pencil, Check, Copy,
  ArrowUp, ArrowDown, Loader2, Search, ChevronDown, Network,
  Shield, Shuffle, Zap, DollarSign, Clock, AlertTriangle,
} from "lucide-react";
import {
  ReactFlow, Handle, Position, Controls,
  useNodesState, useEdgesState,
  type Node, type Edge,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { api, type Chain, type Provider } from "../lib/api";
import { PageHeader } from "../components/Layout";
import { useToast } from "../components/Toast";
import { Card, Button, Input, Field, Badge, Spinner, EmptyState } from "../components/ui";

interface DraftStep {
  provider: string;
  model: string;
}

const isRoundRobinStrategy = (strategy: string) =>
  strategy === "round_robin" || strategy === "round-robin";

const normalizeChainStrategy = (strategy: string) =>
  isRoundRobinStrategy(strategy) ? "round_robin" : strategy;

const displayStrategy = (strategy: string) =>
  isRoundRobinStrategy(strategy) ? "round-robin" : strategy;

// ─── Searchable Select ───────────────────────────────────────────────────────

interface SelectOption {
  value: string;
  label: string;
  sublabel?: string;
  icon?: string;
}

function SearchableSelect({
  options,
  value,
  onChange,
  placeholder = "Select…",
  searchPlaceholder = "Search…",
  disabled = false,
  loading = false,
  className = "",
}: {
  options: SelectOption[];
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  searchPlaceholder?: string;
  disabled?: boolean;
  loading?: boolean;
  className?: string;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const triggerRef = useRef<HTMLButtonElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const [rect, setRect] = useState<DOMRect | null>(null);

  const selected = options.find((o) => o.value === value);

  const filtered = useMemo(() => {
    if (!query.trim()) return options;
    const q = query.toLowerCase();
    return options.filter(
      (o) =>
        o.value.toLowerCase().includes(q) ||
        o.label.toLowerCase().includes(q) ||
        (o.sublabel && o.sublabel.toLowerCase().includes(q)),
    );
  }, [options, query]);

  const updateRect = useCallback(() => {
    if (triggerRef.current) setRect(triggerRef.current.getBoundingClientRect());
  }, []);

  useEffect(() => {
    if (!open) return;
    updateRect();
    const onScroll = () => updateRect();
    const onResize = () => updateRect();
    window.addEventListener("scroll", onScroll, true);
    window.addEventListener("resize", onResize);
    return () => {
      window.removeEventListener("scroll", onScroll, true);
      window.removeEventListener("resize", onResize);
    };
  }, [open, updateRect]);

  useEffect(() => {
    if (!open) return;
    const handleClick = (e: MouseEvent) => {
      const target = e.target as globalThis.Node;
      if (triggerRef.current?.contains(target)) return;
      if (dropdownRef.current?.contains(target)) return;
      setOpen(false);
      setQuery("");
    };
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setOpen(false);
        setQuery("");
      }
    };
    document.addEventListener("mousedown", handleClick);
    document.addEventListener("keydown", handleKey);
    return () => {
      document.removeEventListener("mousedown", handleClick);
      document.removeEventListener("keydown", handleKey);
    };
  }, [open]);

  useEffect(() => {
    if (open && inputRef.current) inputRef.current.focus();
  }, [open]);

  const dropdown = open && rect ? createPortal(
    <div
      ref={dropdownRef}
      className="fixed z-[100] overflow-hidden rounded-xl border border-[var(--border)] bg-[var(--bg-elevated)] shadow-[var(--shadow-float)]"
      style={{
        top: rect.bottom + 4,
        left: rect.left,
        width: Math.max(rect.width, 240),
        maxHeight: 320,
      }}
    >
      <div className="border-b border-[var(--border)] p-2">
        <div className="flex items-center gap-2 rounded-lg bg-[var(--bg-subtle)] px-2.5 py-1.5">
          <Search className="h-3.5 w-3.5 shrink-0 text-[var(--text-muted)]" />
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={searchPlaceholder}
            className="w-full bg-transparent text-sm outline-none placeholder:text-[var(--text-muted)]"
          />
        </div>
      </div>
      <div className="max-h-56 overflow-y-auto p-1">
        {filtered.length === 0 ? (
          <p className="px-3 py-2.5 text-center text-xs text-[var(--text-muted)]">No results</p>
        ) : (
          filtered.map((opt) => (
            <button
              key={opt.value}
              type="button"
              onClick={() => { onChange(opt.value); setOpen(false); setQuery(""); }}
              className={`flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left text-sm transition-colors hover:bg-[var(--bg-subtle)] ${
                opt.value === value ? "bg-accent-500/10 text-accent-600 dark:text-accent-400" : ""
              }`}
            >
              {opt.icon && (
                <img src={opt.icon} alt="" className="h-5 w-5 shrink-0 rounded-sm object-contain" onError={(e) => { (e.target as HTMLImageElement).style.display = "none"; }} />
              )}
              <div className="min-w-0 flex-1">
                <span className="block truncate font-medium">{opt.label}</span>
                {opt.sublabel && <span className="block truncate text-[11px] text-[var(--text-muted)]">{opt.sublabel}</span>}
              </div>
              {opt.value === value && <Check className="h-4 w-4 shrink-0 text-accent-500" />}
            </button>
          ))
        )}
      </div>
    </div>,
    document.body,
  ) : null;

  return (
    <div className={className}>
      <button
        ref={triggerRef}
        type="button"
        disabled={disabled}
        onClick={() => { if (!disabled) { setOpen(!open); setQuery(""); } }}
        className={`flex w-full items-center gap-2 rounded-xl border border-[var(--border)] bg-[var(--bg-elevated)] px-3 py-2 text-left text-sm transition-colors focus:border-accent-400 focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/40 ${
          disabled ? "cursor-not-allowed opacity-50" : "cursor-pointer"
        }`}
      >
        {loading ? (
          <Loader2 className="h-4 w-4 shrink-0 animate-spin text-[var(--text-muted)]" />
        ) : selected?.icon ? (
          <img src={selected.icon} alt="" className="h-5 w-5 shrink-0 rounded-sm object-contain" onError={(e) => { (e.target as HTMLImageElement).style.display = "none"; }} />
        ) : null}
        <span className={`flex-1 truncate ${selected ? "" : "text-[var(--text-muted)]"}`}>
          {selected ? selected.label : placeholder}
        </span>
        <ChevronDown className={`h-4 w-4 shrink-0 text-[var(--text-muted)] transition-transform ${open ? "rotate-180" : ""}`} />
      </button>
      {dropdown}
    </div>
  );
}

// ─── Combo Topology ──────────────────────────────────────────────────────────

function ComboStartNode({ data }: { data: { name: string; strategy: string; stepCount: number } }) {
  return (
    <>
      <Handle type="source" position={Position.Right} className="!bg-accent-500 !border-0 !w-2 !h-2" />
      <div className="flex items-center gap-2.5 rounded-xl border-2 border-accent-500 bg-accent-50 px-4 py-2.5 shadow-sm dark:bg-accent-900/30">
        <Layers className="h-4 w-4 text-accent-600 dark:text-accent-400" />
        <div>
          <span className="block text-sm font-bold text-accent-700 dark:text-accent-300">chain:{data.name}</span>
          <span className="text-[10px] text-[var(--text-muted)]">{data.stepCount} step{data.stepCount !== 1 ? "s" : ""} · {displayStrategy(data.strategy)}</span>
        </div>
      </div>
    </>
  );
}

function ComboStepNode({ data }: { data: { position: number; provider: string; model: string; color: string; icon: string; isLast: boolean; isFallback: boolean } }) {
  return (
    <>
      <Handle type="target" position={Position.Left} className="!bg-accent-500 !border-0 !w-2 !h-2" />
      {!data.isLast && <Handle type="source" position={Position.Right} className="!bg-accent-500 !border-0 !w-2 !h-2" />}
      <div className={`flex items-center gap-2 rounded-lg border-2 bg-[var(--bg-elevated)] px-3 py-2 shadow-sm ${
        data.isFallback ? "border-amber-400 dark:border-amber-500" : ""
      }`} style={{ borderColor: data.isFallback ? undefined : (data.color || "var(--border)") }}>
        {data.isFallback ? (
          <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-amber-500 text-[9px] font-bold text-white">
            <Shield className="h-3 w-3" />
          </span>
        ) : (
          <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-accent-600 text-[9px] font-bold text-white">{data.position + 1}</span>
        )}
        <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md" style={{ backgroundColor: `${data.isFallback ? "#f59e0b" : data.color}15` }}>
          <img src={data.icon} alt={data.provider} className="h-4 w-4 rounded-sm object-contain" onError={(e) => { (e.target as HTMLImageElement).style.display = "none"; }} />
        </div>
        <div className="min-w-0">
          <span className="block text-xs font-medium">{data.provider}</span>
          <span className="block max-w-[140px] truncate font-mono text-[10px] text-[var(--text-muted)]">{data.model}</span>
        </div>
      </div>
    </>
  );
}

const comboNodeTypes = { comboStart: ComboStartNode, comboStep: ComboStepNode };

function ComboTopology({ chain, providers }: { chain: Chain; providers: Provider[] }) {
  const providerMap = useMemo(() => {
    const m = new Map<string, Provider>();
    providers.forEach((p) => m.set(p.id, p));
    return m;
  }, [providers]);

  const { nodes: initialNodes, edges: initialEdges } = useMemo(() => {
    const ns: Node[] = [];
    const es: Edge[] = [];
    const nodeW = 180;
    const gap = 60;
    const allSteps = [...chain.steps];
    const hasFallback = chain.fallback_provider && chain.fallback_model;
    if (hasFallback) {
      allSteps.push({ provider: chain.fallback_provider!, model: chain.fallback_model!, position: allSteps.length });
    }

    ns.push({
      id: "start",
      type: "comboStart",
      position: { x: 0, y: 40 },
      data: { name: chain.name, strategy: chain.strategy, stepCount: chain.steps.length },
      draggable: false,
    });

    allSteps.forEach((step, i) => {
      const p = providerMap.get(step.provider);
      const nodeId = `step-${i}`;
      const isFallback = hasFallback && i === allSteps.length - 1;
      ns.push({
        id: nodeId,
        type: "comboStep",
        position: { x: 220 + i * (nodeW + gap), y: 40 },
        data: {
          position: i,
          provider: step.provider,
          model: step.model,
          color: p?.color || "#6b7280",
          icon: `/providers/${step.provider}.png`,
          isLast: i === allSteps.length - 1,
          isFallback,
        },
        draggable: false,
      });

      const sourceId = i === 0 ? "start" : `step-${i - 1}`;
      es.push({
        id: `e-${sourceId}-${nodeId}`,
        source: sourceId,
        target: nodeId,
        sourceHandle: "right",
        targetHandle: "left",
        animated: i === 0,
        label: isFallback ? "fallback" : i === allSteps.length - 1 ? "last resort" : i > 0 ? `fallback ${i}` : undefined,
        style: { stroke: isFallback ? "#f59e0b" : "var(--color-accent-500)", strokeWidth: 1.5 },
        labelStyle: { fill: "var(--text-muted)", fontSize: 10, fontWeight: 500 },
        labelBgStyle: { fill: "var(--bg-elevated)", fillOpacity: 0.9 },
        labelBgPadding: [6, 3] as [number, number],
        labelBgBorderRadius: 6,
      });
    });

    return { nodes: ns, edges: es };
  }, [chain, providerMap]);

  const [nodes, , onNodesChange] = useNodesState(initialNodes);
  const [edges, , onEdgesChange] = useEdgesState(initialEdges);
  const rfInstance = useRef<any>(null);

  const onInit = useCallback((instance: any) => {
    rfInstance.current = instance;
    setTimeout(() => instance.fitView({ padding: 0.15, duration: 200 }), 50);
  }, []);

  const totalWidth = 220 + (chain.steps.length + (chain.fallback_provider && chain.fallback_model ? 1 : 0)) * 240;
  const height = 140;

  return (
    <div style={{ height, width: "100%", minWidth: Math.min(totalWidth, 600) }}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={comboNodeTypes}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onInit={onInit}
        fitView
        fitViewOptions={{ padding: 0.15 }}
        minZoom={0.4}
        maxZoom={2}
        proOptions={{ hideAttribution: true }}
        panOnDrag
        zoomOnScroll={false}
        zoomOnPinch
        nodesDraggable={false}
        nodesConnectable={false}
        elementsSelectable={false}
      >
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}

// ─── Strategy Cards ──────────────────────────────────────────────────────────

const strategies = [
  { value: "priority", label: "Priority", desc: "Ordered fallback — try step 1 first, then 2, 3…", icon: Zap, color: "text-blue-500" },
  { value: "round_robin", label: "Round Robin", desc: "Rotate across models for even load distribution", icon: Shuffle, color: "text-violet-500" },
  { value: "latency", label: "Latency", desc: "Always try the fastest-responding model first", icon: Clock, color: "text-emerald-500" },
  { value: "cost", label: "Cost", desc: "Route to the cheapest model first, fall back to pricier", icon: DollarSign, color: "text-amber-500" },
];

function StrategyCard({ label, desc, icon: Icon, color, selected, onClick }: {
  label: string; desc: string; icon: any; color: string; selected: boolean; onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`flex items-start gap-3 rounded-xl border-2 p-3 text-left transition-all ${
        selected
          ? "border-accent-500 bg-accent-500/5 dark:bg-accent-500/10"
          : "border-[var(--border)] bg-[var(--bg-elevated)] hover:border-[var(--border)] hover:shadow-sm"
      }`}
    >
      <div className={`mt-0.5 rounded-lg p-1.5 ${selected ? "bg-accent-500/15" : "bg-[var(--bg-subtle)]"}`}>
        <Icon className={`h-4 w-4 ${selected ? "text-accent-600 dark:text-accent-400" : color}`} />
      </div>
      <div className="min-w-0 flex-1">
        <span className={`block text-sm font-semibold ${selected ? "text-accent-700 dark:text-accent-300" : ""}`}>{label}</span>
        <span className="block text-[11px] leading-snug text-[var(--text-muted)]">{desc}</span>
      </div>
      {selected && (
        <div className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-accent-500">
          <Check className="h-3 w-3 text-white" />
        </div>
      )}
    </button>
  );
}

// ─── Main Page ───────────────────────────────────────────────────────────────

export function ChainsPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const chains = useQuery({ queryKey: ["chains"], queryFn: () => api.listChains() });
  const providers = useQuery({ queryKey: ["providers"], queryFn: () => api.providers() });

  const [showModal, setShowModal] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [deletingId, setDeletingId] = useState<string | null>(null);

  const remove = useMutation({
    mutationFn: (id: string) => api.deleteChain(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["chains"] });
      setDeletingId(null);
      toast.success("Combo deleted", "The routing chain has been removed. Target it by name will no longer resolve.");
    },
    onError: (e: Error) => toast.error("Deletion failed", e.message),
  });

  const updateStrategy = useMutation({
    mutationFn: ({ id, strategy }: { id: string; strategy: string }) =>
      api.updateChain(id, { strategy }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["chains"] }),
  });

  const list = chains.data?.chains ?? [];
  const providerList = providers.data?.providers ?? [];

  const openCreate = () => { setEditingId(null); setShowModal(true); };
  const openEdit = (id: string) => { setEditingId(id); setShowModal(true); };
  const closeModal = () => { setShowModal(false); setEditingId(null); };

  return (
    <>
      <PageHeader
        title="Combos"
        icon={Layers}
        description="Named model chains. Target with chain:name or the bare combo name as your model."
        action={
          <Button onClick={openCreate}>
            <Plus className="h-4 w-4" />
            Create Combo
          </Button>
        }
      />

      <div className="space-y-4">
        {deletingId && (
          <Card className="border-red-500/30 bg-red-500/5 dark:border-red-500/20 dark:bg-red-500/10">
            <div className="flex items-center justify-between px-4 py-3">
              <p className="text-sm">
                Delete combo <span className="font-mono font-medium">{list.find((c) => c.id === deletingId)?.name}</span>?
              </p>
              <div className="flex items-center gap-2">
                <Button variant="ghost" onClick={() => setDeletingId(null)} className="h-8 text-xs">Cancel</Button>
                <Button variant="danger" onClick={() => remove.mutate(deletingId)} className="h-8 text-xs"
                  disabled={remove.isPending}>
                  {remove.isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Trash2 className="h-3.5 w-3.5" />}
                  Delete
                </Button>
              </div>
            </div>
          </Card>
        )}

        {chains.isLoading ? (
          <Spinner />
        ) : list.length === 0 ? (
          <Card>
            <EmptyState
              title="No combos yet"
              hint="Create a combo to group models into a named fallback chain."
            />
          </Card>
        ) : (
          <div className="space-y-3">
            {list.map((c) => (
              <ComboCard
                key={c.id}
                chain={c}
                providers={providerList}
                onEdit={() => openEdit(c.id)}
                onDelete={() => setDeletingId(c.id)}
                onToggleRR={() => updateStrategy.mutate({
                  id: c.id,
                  strategy: isRoundRobinStrategy(c.strategy) ? "priority" : "round_robin",
                })}
              />
            ))}
          </div>
        )}
      </div>

      {showModal && (
        <ComboModal
          chain={editingId ? list.find((c) => c.id === editingId) : undefined}
          providers={providerList}
          onClose={closeModal}
        />
      )}
    </>
  );
}

// ─── Combo Card ──────────────────────────────────────────────────────────────

function ComboCard({ chain: c, providers, onEdit, onDelete, onToggleRR }: {
  chain: Chain;
  providers: Provider[];
  onEdit: () => void;
  onDelete: () => void;
  onToggleRR: () => void;
}) {
  const [copied, setCopied] = useState(false);
  const [showAll, setShowAll] = useState(false);
  const [showTopology, setShowTopology] = useState(false);
  const models = c.steps.map((s) => `${s.provider}/${s.model}`);
  const displayModels = showAll ? models : models.slice(0, 3);
  const remaining = models.length - 3;
  const hasFallback = c.fallback_provider && c.fallback_model;

  const providerMap = useMemo(() => {
    const m = new Map<string, { color: string }>();
    providers.forEach((p) => m.set(p.id, { color: p.color }));
    return m;
  }, [providers]);

  const copyName = () => {
    navigator.clipboard.writeText(`chain:${c.name}`);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  return (
    <div className="rounded-xl border border-[var(--border)] bg-[var(--bg)] transition-colors hover:border-[var(--border)] hover:shadow-sm">
      <div className="px-4 py-3">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <Layers className="h-4 w-4 shrink-0 text-accent-500" />
              <span className="font-mono text-sm font-semibold">chain:{c.name}</span>
              <Badge tone="accent">{displayStrategy(c.strategy)}</Badge>
              <span className="text-xs text-[var(--text-muted)]">{models.length} model{models.length !== 1 ? "s" : ""}</span>
              {hasFallback && (
                <span className="inline-flex items-center gap-1 rounded-md bg-amber-500/10 px-1.5 py-0.5 text-[10px] font-medium text-amber-600 dark:text-amber-400">
                  <Shield className="h-3 w-3" /> fallback
                </span>
              )}
            </div>

            <div className="mt-2 flex flex-wrap items-center gap-1.5">
              {displayModels.map((m, i) => {
                const providerId = m.split("/")[0];
                const color = providerMap.get(providerId)?.color;
                return (
                  <span key={i} className="flex items-center">
                    {i > 0 && <ArrowRight className="mx-0.5 h-3 w-3 text-[var(--text-muted)]" />}
                    <span className="flex items-center gap-1.5 rounded-md bg-[var(--bg-subtle)] px-2 py-0.5" style={color ? { borderLeft: `2px solid ${color}` } : undefined}>
                      <img src={`/providers/${providerId}.png`} alt="" className="h-3 w-3 rounded-sm object-contain" onError={(e) => { (e.target as HTMLImageElement).style.display = "none"; }} />
                      <span className="font-mono text-[11px] text-[var(--text-muted)]">{m}</span>
                    </span>
                  </span>
                );
              })}
              {!showAll && remaining > 0 && (
                <button onClick={() => setShowAll(true)}
                  className="rounded-md bg-[var(--bg-subtle)] px-2 py-0.5 text-[11px] text-[var(--text-muted)] hover:bg-[var(--bg-elevated)]">
                  +{remaining} more
                </button>
              )}
              {showAll && models.length > 3 && (
                <button onClick={() => setShowAll(false)}
                  className="text-[11px] text-[var(--text-muted)] hover:text-[var(--text)]">
                  show less
                </button>
              )}
              {hasFallback && (
                <>
                  <ArrowRight className="mx-0.5 h-3 w-3 text-amber-400" />
                  <span className="flex items-center gap-1.5 rounded-md border border-amber-300/40 bg-amber-500/5 px-2 py-0.5 dark:bg-amber-500/10">
                    <Shield className="h-3 w-3 text-amber-500" />
                    <span className="font-mono text-[11px] text-amber-600 dark:text-amber-400">{c.fallback_provider}/{c.fallback_model}</span>
                  </span>
                </>
              )}
            </div>
          </div>

          <div className="flex shrink-0 items-center gap-0.5">
            <button onClick={() => setShowTopology(!showTopology)}
              className={`flex h-7 items-center gap-1 rounded-lg border px-2 text-[10px] font-medium transition-colors ${
                showTopology
                  ? "border-accent-500/40 bg-accent-500/10 text-accent-600 dark:text-accent-400"
                  : "border-[var(--border)] text-[var(--text-muted)] hover:bg-[var(--bg-subtle)]"
              }`}
              title="Show topology">
              <Network className="h-3 w-3" />
            </button>
            <button onClick={onToggleRR}
              className={`flex h-7 items-center gap-1 rounded-lg border px-2 text-[10px] font-medium transition-colors ${
                isRoundRobinStrategy(c.strategy)
                  ? "border-accent-500/40 bg-accent-500/10 text-accent-600 dark:text-accent-400"
                  : "border-[var(--border)] text-[var(--text-muted)] hover:bg-[var(--bg-subtle)]"
              }`}
              title="Toggle round-robin">
              RR
            </button>
            <button onClick={copyName}
              className="rounded-lg p-1.5 text-[var(--text-muted)] hover:bg-[var(--bg-subtle)] hover:text-[var(--text)]"
              title="Copy combo name">
              {copied ? <Check className="h-4 w-4 text-emerald-500 dark:text-emerald-400" /> : <Copy className="h-4 w-4" />}
            </button>
            <button onClick={onEdit}
              className="rounded-lg p-1.5 text-[var(--text-muted)] hover:bg-[var(--bg-subtle)] hover:text-[var(--text)]"
              title="Edit">
              <Pencil className="h-4 w-4" />
            </button>
            <button onClick={onDelete}
              className="rounded-lg p-1.5 text-[var(--text-muted)] hover:bg-red-500/10 hover:text-red-500"
              title="Delete">
              <Trash2 className="h-4 w-4" />
            </button>
          </div>
        </div>
      </div>

      {showTopology && (
        <div className="border-t border-[var(--border)] px-4 py-3">
          <ComboTopology chain={c} providers={providers} />
        </div>
      )}
    </div>
  );
}

// ─── Combo Modal (Create / Edit) ─────────────────────────────────────────────

function ComboModal({ chain, providers, onClose }: {
  chain?: Chain;
  providers: Provider[];
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const toast = useToast();
  const isEdit = !!chain;

  const [name, setName] = useState(chain?.name ?? "");
  const [strategy, setStrategy] = useState(normalizeChainStrategy(chain?.strategy ?? "priority"));
  const [steps, setSteps] = useState<DraftStep[]>(
    chain?.steps.map((s) => ({ provider: s.provider, model: s.model })) ?? [{ provider: "", model: "" }]
  );
  const [fallbackProvider, setFallbackProvider] = useState(chain?.fallback_provider ?? "");
  const [fallbackModel, setFallbackModel] = useState(chain?.fallback_model ?? "");
  const [error, setError] = useState("");

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  const create = useMutation({
    mutationFn: () => api.createChain({
      name: name.trim(),
      strategy,
      steps: steps.filter((s) => s.provider && s.model),
      fallback_provider: fallbackProvider || undefined,
      fallback_model: fallbackModel || undefined,
    }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["chains"] });
      toast.success("Combo created", `Routing chain "${name.trim()}" is now available as a model target.`);
      onClose();
    },
    onError: (e: Error) => {
      setError(e.message);
      toast.error("Combo creation failed", e.message);
    },
  });

  const update = useMutation({
    mutationFn: () => api.updateChain(chain!.id, {
      name: name.trim(),
      strategy,
      steps: steps.filter((s) => s.provider && s.model),
      fallback_provider: fallbackProvider || undefined,
      fallback_model: fallbackModel || undefined,
    }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["chains"] });
      toast.success("Combo updated", `Routing chain "${name.trim()}" has been saved with the new configuration.`);
      onClose();
    },
    onError: (e: Error) => {
      setError(e.message);
      toast.error("Combo update failed", e.message);
    },
  });

  const addStep = () => setSteps((s) => [...s, { provider: "", model: "" }]);
  const removeStep = (i: number) => setSteps((s) => s.filter((_, idx) => idx !== i));
  const updateStep = (i: number, patch: Partial<DraftStep>) =>
    setSteps((prev) => prev.map((s, idx) => (idx === i ? { ...s, ...patch } : s)));
  const moveStep = (i: number, dir: -1 | 1) => {
    setSteps((prev) => {
      const next = [...prev];
      const j = i + dir;
      if (j < 0 || j >= next.length) return prev;
      [next[i], next[j]] = [next[j], next[i]];
      return next;
    });
  };

  const valid = name.trim() && steps.some((s) => s.provider && s.model);

  // Fallback model options
  const fallbackProviderOptions: SelectOption[] = providers.map((p) => ({
    value: p.id,
    label: p.display_name,
    sublabel: p.id,
    icon: `/providers/${p.id}.png`,
  }));

  const fallbackModelsQuery = useQuery({
    queryKey: ["providerModels", fallbackProvider],
    queryFn: () => api.providerModels(fallbackProvider),
    enabled: !!fallbackProvider,
    staleTime: 60_000,
  });

  const fallbackModelOptions: SelectOption[] = (fallbackModelsQuery.data?.models ?? []).map((m) => ({
    value: m.id,
    label: m.name || m.id,
    sublabel: m.id !== m.name ? m.id : undefined,
  }));

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4 backdrop-blur-sm"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
    >
      <div
        className="flex w-full max-w-2xl max-h-[90vh] flex-col rounded-2xl border border-[var(--border)] bg-[var(--bg-elevated)] shadow-[var(--shadow-float)]"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between border-b border-[var(--border)] px-6 py-4">
          <h2 className="text-base font-semibold">{isEdit ? "Edit Combo" : "Create Combo"}</h2>
          <button onClick={onClose} className="rounded-lg p-1.5 text-[var(--text-muted)] hover:text-[var(--text)]">
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto space-y-6 px-6 py-5">
          {/* Name */}
          <Field label="Combo name">
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="my-combo"
              className="font-mono" />
            <p className="mt-1 text-[10px] text-[var(--text-muted)]">
              Use as <span className="font-mono">chain:{name || "name"}</span> or bare <span className="font-mono">{name || "name"}</span> as model.
              Letters, digits, hyphens, underscores only.
            </p>
          </Field>

          {/* Strategy Cards */}
          <div>
            <span className="text-xs font-medium text-[var(--text-muted)]">Strategy</span>
            <div className="mt-2 grid grid-cols-1 gap-2 sm:grid-cols-2">
              {strategies.map((s) => (
                <StrategyCard
                  key={s.value}
                  {...s}
                  selected={strategy === s.value}
                  onClick={() => setStrategy(s.value)}
                />
              ))}
            </div>
          </div>

          {/* Steps */}
          <div>
            <span className="text-xs font-medium text-[var(--text-muted)]">Model chain</span>
            <p className="mb-2 text-[10px] text-[var(--text-muted)]">Models are tried in order from top to bottom based on the selected strategy.</p>
            <div className="space-y-1">
              {steps.map((step, i) => (
                <StepRow
                  key={i}
                  index={i}
                  step={step}
                  total={steps.length}
                  providers={providers}
                  onUpdate={(patch) => updateStep(i, patch)}
                  onRemove={() => removeStep(i)}
                  onMoveUp={i > 0 ? () => moveStep(i, -1) : undefined}
                  onMoveDown={i < steps.length - 1 ? () => moveStep(i, 1) : undefined}
                />
              ))}
            </div>
            <button onClick={addStep}
              className="mt-2 flex w-full items-center justify-center gap-2 rounded-xl border-2 border-dashed border-[var(--border)] py-2.5 text-xs font-medium text-[var(--text-muted)] transition-colors hover:border-accent-400 hover:text-accent-600 dark:hover:text-accent-400">
              <Plus className="h-4 w-4" /> Add model
            </button>
          </div>

          {/* Fallback Model Section */}
          <div className="rounded-xl border border-amber-300/40 bg-amber-50/50 p-4 dark:bg-amber-500/5">
            <div className="flex items-center gap-2 mb-3">
              <div className="rounded-lg bg-amber-500/15 p-1.5">
                <Shield className="h-4 w-4 text-amber-600 dark:text-amber-400" />
              </div>
              <div>
                <span className="block text-sm font-semibold text-amber-800 dark:text-amber-300">Fallback model</span>
                <span className="block text-[11px] text-amber-600/80 dark:text-amber-400/70">Last resort when all chain steps fail. Optional.</span>
              </div>
            </div>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <SearchableSelect
                options={fallbackProviderOptions}
                value={fallbackProvider}
                onChange={(v) => { setFallbackProvider(v); setFallbackModel(""); }}
                placeholder="Provider…"
                searchPlaceholder="Search providers…"
              />
              <SearchableSelect
                options={fallbackModelOptions}
                value={fallbackModel}
                onChange={(v) => setFallbackModel(v)}
                placeholder={fallbackProvider ? "Model…" : "Select provider first"}
                searchPlaceholder="Search models…"
                disabled={!fallbackProvider}
                loading={fallbackModelsQuery.isLoading}
              />
            </div>
          </div>

          {error && (
            <div className="flex items-center gap-2 rounded-lg border border-red-300 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-400">
              <AlertTriangle className="h-3.5 w-3.5 shrink-0" />
              {error}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center gap-2 border-t border-[var(--border)] px-6 py-4">
          <Button onClick={() => (isEdit ? update.mutate() : create.mutate())}
            disabled={!valid || create.isPending || update.isPending}>
            {(create.isPending || update.isPending) ? <Loader2 className="h-4 w-4 animate-spin" /> : <Check className="h-4 w-4" />}
            {isEdit ? "Save changes" : "Create combo"}
          </Button>
          <Button variant="ghost" onClick={onClose}>Cancel</Button>
        </div>
      </div>
    </div>
  );
}

// ─── Step Row ────────────────────────────────────────────────────────────────

function StepRow({
  index,
  step,
  total,
  providers,
  onUpdate,
  onRemove,
  onMoveUp,
  onMoveDown,
}: {
  index: number;
  step: DraftStep;
  total: number;
  providers: Provider[];
  onUpdate: (patch: Partial<DraftStep>) => void;
  onRemove: () => void;
  onMoveUp?: () => void;
  onMoveDown?: () => void;
}) {
  const modelsQuery = useQuery({
    queryKey: ["providerModels", step.provider],
    queryFn: () => api.providerModels(step.provider),
    enabled: !!step.provider,
    staleTime: 60_000,
  });

  const providerOptions: SelectOption[] = providers.map((p) => ({
    value: p.id,
    label: p.display_name,
    sublabel: p.id,
    icon: `/providers/${p.id}.png`,
  }));

  const modelOptions: SelectOption[] = (modelsQuery.data?.models ?? []).map((m) => ({
    value: m.id,
    label: m.name || m.id,
    sublabel: m.id !== m.name ? m.id : undefined,
  }));

  const providerColor = step.provider ? providers.find((p) => p.id === step.provider)?.color : undefined;

  return (
    <div>
      {index > 0 && (
        <div className="flex items-center py-1 pl-8">
          <div className="h-4 w-px bg-[var(--border)]" />
          <ArrowRight className="h-3 w-3 -ml-1.5 text-[var(--text-muted)]" />
          <span className="ml-1 text-[10px] text-[var(--text-muted)]">
            {index === total - 1 ? "last resort" : `fallback ${index}`}
          </span>
        </div>
      )}
      <div className="flex items-center gap-2 rounded-xl border border-[var(--border)] bg-[var(--bg-elevated)] p-3 transition-colors hover:shadow-sm"
        style={providerColor ? { borderLeft: `3px solid ${providerColor}` } : undefined}
      >
        {/* Step number */}
        <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-xs font-bold text-white"
          style={{ backgroundColor: providerColor || "var(--color-accent-600, #6366f1)" }}>
          {index + 1}
        </span>

        {/* Provider + model selects */}
        <div className="flex flex-1 flex-col gap-2 sm:flex-row sm:items-center">
          <SearchableSelect
            options={providerOptions}
            value={step.provider}
            onChange={(v) => onUpdate({ provider: v, model: "" })}
            placeholder="Provider…"
            searchPlaceholder="Search providers…"
            className="w-full sm:w-48"
          />
          <SearchableSelect
            options={modelOptions}
            value={step.model}
            onChange={(v) => onUpdate({ model: v })}
            placeholder={step.provider ? "Model…" : "Select provider first"}
            searchPlaceholder="Search models…"
            disabled={!step.provider}
            loading={modelsQuery.isLoading}
            className="flex-1 min-w-0"
          />
        </div>

        {/* Actions */}
        <div className="flex shrink-0 items-center gap-0.5">
          {onMoveUp && (
            <button onClick={onMoveUp}
              className="rounded p-1 text-[var(--text-muted)] hover:bg-[var(--bg-subtle)]">
              <ArrowUp className="h-3.5 w-3.5" />
            </button>
          )}
          {onMoveDown && (
            <button onClick={onMoveDown}
              className="rounded p-1 text-[var(--text-muted)] hover:bg-[var(--bg-subtle)]">
              <ArrowDown className="h-3.5 w-3.5" />
            </button>
          )}
          {total > 1 && (
            <button onClick={onRemove}
              className="rounded p-1 text-[var(--text-muted)] hover:bg-red-500/10 hover:text-red-500">
              <X className="h-3.5 w-3.5" />
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
