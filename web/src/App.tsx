import { useEffect, useState } from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import {
  ArrowDownToLine,
  Boxes,
  Database,
  ExternalLink,
  Gauge,
  KeyRound,
  Layers3,
  RefreshCw,
  ShieldCheck,
  Sparkles,
  TriangleAlert,
  Zap,
} from "lucide-react"
import { toast } from "sonner"

import { api, compactNumber, formatNumber } from "@/lib/api"
import { KeysPage } from "@/components/keys-page"
import { ModelsPage } from "@/components/models-page"
import { RequestLog } from "@/components/request-log"
import { StatCard } from "@/components/stat-card"
import { UsagePanel } from "@/components/usage-panel"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { cn } from "cn"

type View = "dashboard" | "keys" | "models"

function currentView(): View {
  const hash = window.location.hash.replace("#", "")
  if (hash === "keys" || hash === "models") return hash
  return "dashboard"
}

function App() {
  const queryClient = useQueryClient()
  const [view, setView] = useState<View>(currentView)

  const status = useQuery({
    queryKey: ["status"],
    queryFn: api.status,
    refetchInterval: 10_000,
    retry: 1,
  })
  const usage = useQuery({
    queryKey: ["usage"],
    queryFn: api.usage,
    refetchInterval: 3_000,
    retry: 1,
  })
  useEffect(() => {
    const sync = () => setView(currentView())
    window.addEventListener("hashchange", sync)
    return () => window.removeEventListener("hashchange", sync)
  }, [])

  const navigate = (next: View) => {
    window.location.hash = next === "dashboard" ? "" : next
    setView(next)
  }

  const refreshing = status.isFetching || usage.isFetching
  const totals = usage.data?.totals
  const avgTps =
    totals && (totals.latencyMs ?? 0) > 0
      ? totals.completionTokens / ((totals.latencyMs ?? 0) / 1000)
      : 0
  const avgTpsLabel =
    totals && (totals.latencyMs ?? 0) > 0
      ? avgTps >= 100
        ? formatNumber(Math.round(avgTps))
        : avgTps.toFixed(1)
      : "—"

  const refresh = async () => {
    await queryClient.invalidateQueries()
    toast.success("Dashboard refreshed")
  }

  return (
    <div className="relative min-h-screen">
      <div className="aurora" />
      <div className="grid-overlay" />

      <div className="relative z-10 mx-auto flex min-h-screen max-w-7xl flex-col px-4 pb-10 sm:px-6 lg:px-8">
        <header className="flex items-center justify-between gap-4 py-6">
          <div className="flex items-center gap-3">
            <div className="relative flex size-10 items-center justify-center rounded-xl bg-gradient-to-br from-primary via-fuchsia-500 to-accent shadow-lg shadow-primary/25">
              <Layers3 className="size-5 text-white" />
              <span className="absolute -right-0.5 -bottom-0.5 size-2.5 rounded-full border-2 border-background bg-emerald-400" />
            </div>
            <div>
              <p className="text-sm font-semibold tracking-tight">Vibe API</p>
              <p className="text-[11px] text-muted-foreground">Mistral control plane</p>
            </div>
          </div>

          <nav className="hidden items-center gap-1 rounded-lg border border-border/60 bg-background/30 p-1 lg:flex">
            {(["dashboard", "keys", "models"] as const).map((item) => (
              <button
                key={item}
                type="button"
                onClick={() => navigate(item)}
                className={cn(
                  "rounded-md px-3 py-1 text-xs font-medium transition-colors",
                  view === item
                    ? "bg-secondary text-foreground shadow-sm"
                    : "text-muted-foreground hover:text-foreground",
                )}
              >
                {item === "dashboard" ? "Overview" : item === "keys" ? "API keys" : "Models"}
              </button>
            ))}
          </nav>

          <div className="flex items-center gap-2">
            {status.data?.storage === "postgres" ? (
              <Badge
                variant="outline"
                className="hidden h-8 gap-1.5 border-border/70 bg-background/30 px-2.5 font-normal text-[11px] text-muted-foreground sm:inline-flex"
                title="Key and usage history persisted in PostgreSQL"
              >
                <Database className="size-3 text-emerald-400" />
                Postgres
              </Badge>
            ) : null}
            <Badge
              variant="outline"
              className="hidden h-8 gap-1.5 border-border/70 bg-background/30 px-2.5 font-normal text-[11px] text-muted-foreground sm:inline-flex"
            >
              <span
                className={
                  status.data?.keyConfigured
                    ? "size-1.5 rounded-full bg-emerald-400"
                    : "size-1.5 rounded-full bg-amber-400"
                }
              />
              {status.data?.keyConfigured ? "Connected" : "Needs API key"}
            </Badge>
            <Button
              variant="outline"
              size="sm"
              aria-label="Models"
              className="h-8 border-border/70 bg-background/30 px-2 lg:hidden"
              onClick={() => navigate("models")}
            >
              <Boxes />
            </Button>
            <Button
              variant="outline"
              size="sm"
              aria-label="Refresh dashboard"
              className="h-8 border-border/70 bg-background/30"
              onClick={() => void refresh()}
            >
              <RefreshCw className={refreshing ? "animate-spin" : undefined} />
              <span className="hidden sm:inline">Refresh</span>
            </Button>
            <Button
              size="sm"
              aria-label="API keys"
              className="h-8 shadow-lg shadow-primary/25"
              onClick={() => navigate("keys")}
            >
              <KeyRound />
              <span className="hidden sm:inline">API keys</span>
            </Button>
          </div>
        </header>

        <main className="flex-1 space-y-5">
          {view === "keys" ? (
            <KeysPage onBack={() => navigate("dashboard")} />
          ) : view === "models" ? (
            <ModelsPage
              enabled={status.data?.keyConfigured === true}
              onManageKeys={() => navigate("keys")}
            />
          ) : (
            <>
          <section className="animate-rise flex flex-col justify-between gap-5 pt-2 pb-1 lg:flex-row lg:items-end">
            <div className="max-w-3xl">
              <div className="inline-flex items-center gap-2 rounded-full border border-primary/25 bg-primary/10 px-3 py-1 text-[10px] font-medium tracking-[0.18em] text-primary uppercase">
                <Sparkles className="size-3" />
                Control plane
              </div>
              <h1 className="text-gradient mt-4 text-4xl font-semibold tracking-tight text-balance sm:text-5xl">
                One gateway.
                <br className="hidden sm:block" /> Every Mistral model.
              </h1>
              <p className="mt-3 max-w-xl text-sm leading-relaxed text-muted-foreground">
                OpenAI and Anthropic-compatible endpoints with live model discovery, runtime key rotation,
                and token accounting — no restarts.
              </p>
            </div>

            <div className="glass flex shrink-0 items-center gap-4 rounded-2xl px-4 py-3 text-xs">
              <div className="space-y-1">
                <p className="text-[10px] tracking-wider text-muted-foreground uppercase">Upstream</p>
                <p className="font-mono text-[11px] text-foreground/85">{status.data?.upstream ?? "—"}</p>
              </div>
              <div className="h-8 w-px bg-border" />
              <div className="space-y-1">
                <p className="text-[10px] tracking-wider text-muted-foreground uppercase">Anthropic model</p>
                <p className="font-mono text-[11px] text-foreground/85">{status.data?.anthropicModel ?? "—"}</p>
              </div>
              {status.data?.keyConfigured ? (
                <div className="flex items-center gap-1.5 text-[11px] text-emerald-400">
                  <ShieldCheck className="size-3.5" />
                  {status.data.keyHint}
                </div>
              ) : null}
            </div>
          </section>

          {status.isError ? (
            <div className="flex items-center gap-2 rounded-xl border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-red-300">
              <TriangleAlert className="size-4 shrink-0" />
              Could not reach the proxy API: {(status.error as Error).message}
            </div>
          ) : null}

          {status.data && !status.data.keyConfigured ? (
            <button
              onClick={() => navigate("keys")}
              className="group flex w-full items-center gap-3 rounded-2xl border border-amber-400/25 bg-amber-400/[0.07] px-4 py-3 text-left transition-colors hover:bg-amber-400/10"
            >
              <span className="flex size-9 items-center justify-center rounded-xl bg-amber-400/15 text-amber-300 ring-1 ring-amber-400/25">
                <Zap className="size-4" />
              </span>
              <span className="min-w-0 flex-1">
                <span className="block text-sm font-medium text-amber-100">Connect your Mistral account</span>
                <span className="block truncate text-xs text-amber-200/60">
                  Add an API key to load the model catalog and start tracking usage.
                </span>
              </span>
              <span className="shrink-0 text-xs font-medium text-amber-200/80 transition-transform group-hover:translate-x-0.5">
                Add key →
              </span>
            </button>
          ) : null}

          <div className="flex flex-col gap-1 sm:flex-row sm:items-end sm:justify-between">
            <div>
              <h2 className="text-lg font-semibold tracking-tight">Usage</h2>
              <p className="text-xs text-muted-foreground">
                Token accounting, throughput, and latency across every proxied call.
              </p>
            </div>
            <span className="text-[10px] tracking-wider text-muted-foreground uppercase">
              Refreshes every 3 seconds
            </span>
          </div>

          <section className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-5">
            <StatCard
              label="Requests"
              value={totals ? formatNumber(totals.requests) : "—"}
              detail={
                status.data?.storage === "postgres" ? "Persisted across restarts" : "Since proxy start"
              }
              icon={Layers3}
              tone="violet"
            />
            <StatCard
              label="Total tokens"
              value={totals ? compactNumber(totals.totalTokens) : "—"}
              detail={`${compactNumber(totals?.promptTokens ?? 0)} in · ${compactNumber(totals?.completionTokens ?? 0)} out`}
              icon={Database}
              tone="cyan"
              delay={40}
            />
            <StatCard
              label="Cached tokens"
              value={totals ? compactNumber(totals.cachedTokens ?? 0) : "—"}
              detail="Served from cache"
              icon={ArrowDownToLine}
              tone="emerald"
              delay={80}
            />
            <StatCard
              label="Reasoning tokens"
              value={totals ? compactNumber(totals.reasoningTokens ?? 0) : "—"}
              detail="Reasoning model output"
              icon={Sparkles}
              tone="amber"
              delay={120}
            />
            <div className="col-span-2 sm:col-span-1">
              <StatCard
                label="Avg TPS / request"
                value={avgTpsLabel}
                detail="Output tokens per wall-clock second"
                icon={Gauge}
                tone="violet"
                delay={160}
              />
            </div>
          </section>

          <UsagePanel snapshot={usage.data} pending={usage.isPending} />

          <RequestLog snapshot={usage.data} />
            </>
          )}
        </main>

        <footer className="mt-8 flex flex-col items-center justify-between gap-3 border-t border-border/50 pt-5 text-[11px] text-muted-foreground sm:flex-row">
          <p>
            Vibe API · OpenAI and Anthropic wire formats ·{" "}
            {status.data?.storage === "postgres"
              ? "key and usage persisted in Postgres"
              : "usage is in-memory and resets on restart"}
          </p>
          <div className="flex items-center gap-4">
            <a className="inline-flex items-center gap-1 hover:text-foreground" href="/healthz" target="_blank" rel="noreferrer">
              Health <ExternalLink className="size-3" />
            </a>
            <a className="inline-flex items-center gap-1 hover:text-foreground" href="/v1/models" target="_blank" rel="noreferrer">
              Models API <ExternalLink className="size-3" />
            </a>
          </div>
        </footer>
      </div>

    </div>
  )
}

export default App
