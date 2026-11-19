import { useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import {
  ArrowUpRight,
  Boxes,
  BrainCircuit,
  Cpu,
  Eye,
  FunctionSquare,
  KeyRound,
  Loader2,
  Search,
  TriangleAlert,
} from "lucide-react"

import { api, formatNumber, type Model } from "@/lib/api"
import { ModelCard, capabilityMeta, contextLabel } from "@/components/model-card"
import { StatCard } from "@/components/stat-card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "cn"

type Props = {
  enabled: boolean
  onManageKeys: () => void
}

type Filter = "all" | "chat" | "tools" | "vision" | "reasoning"

const filters: { key: Filter; label: string; icon: typeof Cpu }[] = [
  { key: "all", label: "All", icon: Boxes },
  { key: "chat", label: "Chat", icon: Cpu },
  { key: "tools", label: "Tools", icon: FunctionSquare },
  { key: "vision", label: "Vision", icon: Eye },
  { key: "reasoning", label: "Reasoning", icon: BrainCircuit },
]

function matchesFilter(model: Model, filter: Filter): boolean {
  if (filter === "all") return true
  const capabilities = model.capabilities ?? {}
  if (filter === "tools") return capabilities.function_calling === true
  if (filter === "vision") return capabilities.vision === true
  if (filter === "reasoning") return capabilities.reasoning === true
  return capabilities.completion_chat === true
}

function hasCapability(model: Model, capability: string) {
  return model.capabilities?.[capability] === true
}

export function ModelsPage({ enabled, onManageKeys }: Props) {
  const [search, setSearch] = useState("")
  const [filter, setFilter] = useState<Filter>("all")
  const models = useQuery({
    queryKey: ["models"],
    queryFn: api.models,
    enabled,
    refetchInterval: 60_000,
    retry: 1,
  })

  const all = models.data?.data ?? []
  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase()
    return all.filter((model) => {
      if (!matchesFilter(model, filter)) return false
      if (!query) return true
      return [model.id, model.name, model.description].some((value) =>
        value?.toLowerCase().includes(query),
      )
    })
  }, [all, filter, search])

  const tools = all.filter((model) => hasCapability(model, "function_calling")).length
  const vision = all.filter((model) => hasCapability(model, "vision")).length
  const reasoning = all.filter((model) => hasCapability(model, "reasoning")).length
  const largestContext = all.reduce((largest, model) => Math.max(largest, model.max_context_length ?? 0), 0)

  return (
    <>
      <section className="animate-rise flex flex-col justify-between gap-4 pt-2 pb-1 lg:flex-row lg:items-end">
        <div>
          <div className="inline-flex items-center gap-2 rounded-full border border-accent/25 bg-accent/10 px-3 py-1 text-[10px] font-medium tracking-[0.18em] text-accent uppercase">
            <Boxes className="size-3" />
            Live catalog
          </div>
          <h1 className="text-gradient mt-4 text-4xl font-semibold tracking-tight sm:text-5xl">Models</h1>
          <p className="mt-3 max-w-xl text-sm leading-relaxed text-muted-foreground">
            Every model exposed by <span className="font-mono text-foreground/80">GET /v1/models</span>,
            with capabilities, context windows, and copyable ids.
          </p>
        </div>
        <div className="flex items-center gap-3">
          <div className="glass rounded-xl px-4 py-2.5 text-xs">
            <p className="text-[10px] tracking-wider text-muted-foreground uppercase">Available</p>
            <p className="mt-0.5 font-mono text-sm text-foreground">
              {models.data ? formatNumber(all.length) : "—"}
            </p>
          </div>
          <Button
            variant="outline"
            size="sm"
            className="h-9 border-border/70 bg-background/30"
            onClick={() => void models.refetch()}
            disabled={!enabled || models.isFetching}
          >
            {models.isFetching ? <Loader2 className="animate-spin" /> : <Boxes />}
            Refresh
          </Button>
        </div>
      </section>

      {!enabled ? (
        <section className="glass flex flex-col items-center gap-3 rounded-2xl px-6 py-16 text-center">
          <span className="flex size-12 items-center justify-center rounded-2xl bg-amber-400/10 text-amber-300 ring-1 ring-amber-400/25">
            <KeyRound className="size-5" />
          </span>
          <div>
            <h2 className="text-base font-semibold">Connect a key to load the catalog</h2>
            <p className="mx-auto mt-1 max-w-md text-sm text-muted-foreground">
              Models are fetched from Mistral using the enabled keys in the proxy pool.
            </p>
          </div>
          <Button onClick={onManageKeys}>
            <KeyRound />
            Manage API keys
          </Button>
        </section>
      ) : models.isError ? (
        <section className="glass flex flex-col items-center gap-3 rounded-2xl px-6 py-16 text-center">
          <TriangleAlert className="size-6 text-destructive" />
          <div>
            <h2 className="text-base font-semibold">Could not load the catalog</h2>
            <p className="mx-auto mt-1 max-w-lg text-sm text-muted-foreground">
              {(models.error as Error).message}
            </p>
          </div>
          <Button variant="outline" onClick={() => void models.refetch()}>
            Retry
          </Button>
        </section>
      ) : (
        <>
          <section className="grid grid-cols-2 gap-3 xl:grid-cols-4">
            <StatCard
              label="Models"
              value={models.data ? formatNumber(all.length) : "…"}
              detail="In the live catalog"
              icon={Boxes}
              tone="violet"
            />
            <StatCard
              label="Tool calling"
              value={models.data ? formatNumber(tools) : "…"}
              detail="Support function calling"
              icon={FunctionSquare}
              tone="cyan"
              delay={40}
            />
            <StatCard
              label="Vision"
              value={models.data ? formatNumber(vision) : "…"}
              detail="Accept images"
              icon={Eye}
              tone="emerald"
              delay={80}
            />
            <StatCard
              label="Largest context"
              value={models.data ? contextLabel(largestContext).replace(" context", "") : "…"}
              detail={`${formatNumber(reasoning)} reasoning models`}
              icon={Cpu}
              tone="amber"
              delay={120}
            />
          </section>

          <section className="glass rounded-2xl p-4">
            <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div className="flex flex-wrap items-center gap-1.5">
                {filters.map((option) => {
                  const Icon = option.icon
                  const active = filter === option.key
                  return (
                    <Button
                      key={option.key}
                      type="button"
                      variant={active ? "secondary" : "ghost"}
                      size="sm"
                      className={cn(
                        "h-8 gap-1.5 px-3 text-xs",
                        active
                          ? "border border-border/70 bg-secondary/80 text-foreground"
                          : "text-muted-foreground",
                      )}
                      onClick={() => setFilter(option.key)}
                    >
                      <Icon className="size-3.5" />
                      {option.label}
                    </Button>
                  )
                })}
              </div>
              <div className="relative w-full lg:w-80">
                <Search className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  placeholder="Search id, name, or description…"
                  className="h-9 bg-background/40 pl-9"
                  value={search}
                  onChange={(event) => setSearch(event.target.value)}
                />
              </div>
            </div>
          </section>

          {models.isPending ? (
            <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
              {Array.from({ length: 9 }).map((_, index) => (
                <Skeleton key={index} className="h-40 rounded-2xl" />
              ))}
            </div>
          ) : filtered.length === 0 ? (
            <section className="glass rounded-2xl px-6 py-16 text-center">
              <Search className="mx-auto size-6 text-muted-foreground/60" />
              <p className="mt-3 text-sm font-medium">No models match your filters</p>
              <p className="mt-1 text-xs text-muted-foreground">
                Try a different capability or clear the search.
              </p>
            </section>
          ) : (
            <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
              {filtered.map((model) => (
                <ModelCard key={`${model.id}-${model.name ?? ""}`} model={model} allCapabilities />
              ))}
            </div>
          )}

          <div className="flex flex-col items-start justify-between gap-2 text-[11px] text-muted-foreground sm:flex-row sm:items-center">
            <span>
              Showing {filtered.length} of {all.length} models
              {filter !== "all" ? ` · ${filters.find((item) => item.key === filter)?.label}` : ""}
            </span>
            <div className="flex items-center gap-4">
              <span>
                Capabilities: {Object.values(capabilityMeta).length} tracked
              </span>
              <a
                className="inline-flex items-center gap-1 transition-colors hover:text-foreground"
                href="/v1/models"
                target="_blank"
                rel="noreferrer"
              >
                Raw JSON <ArrowUpRight className="size-3" />
              </a>
            </div>
          </div>
        </>
      )}
    </>
  )
}
