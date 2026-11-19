import { useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import {
  ArrowUpRight,
  BrainCircuit,
  Check,
  Copy,
  Cpu,
  Eye,
  FunctionSquare,
  Search,
  Sparkles,
  TriangleAlert,
} from "lucide-react"
import { toast } from "sonner"

import { api, type Model } from "@/lib/api"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"

const capabilityMeta: Record<string, { label: string; icon: typeof Cpu }> = {
  completion_chat: { label: "Chat", icon: Sparkles },
  function_calling: { label: "Tools", icon: FunctionSquare },
  vision: { label: "Vision", icon: Eye },
  reasoning: { label: "Reasoning", icon: BrainCircuit },
  completion_fim: { label: "FIM", icon: Cpu },
  fine_tuning: { label: "Fine-tune", icon: Cpu },
  ocr: { label: "OCR", icon: Cpu },
}

function contextLabel(value?: number) {
  if (!value) return "—"
  if (value >= 1000) return `${Math.round(value / 1000)}K context`
  return `${value} context`
}

function ModelCard({ model }: { model: Model }) {
  const [copied, setCopied] = useState(false)
  const capabilities = Object.entries(model.capabilities ?? {})
    .filter(([key, enabled]) => enabled && capabilityMeta[key])
    .slice(0, 4)

  return (
    <Card className="group relative overflow-hidden border-border/60 bg-card/50 transition-all duration-300 hover:-translate-y-0.5 hover:border-primary/30 hover:shadow-[0_20px_50px_-24px_rgba(139,92,246,0.55)]">
      <div className="pointer-events-none absolute inset-x-0 -top-24 h-32 bg-gradient-to-b from-primary/15 to-transparent opacity-0 transition-opacity duration-300 group-hover:opacity-100" />
      <CardHeader className="gap-1.5">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <CardTitle className="truncate text-[0.95rem] font-semibold tracking-tight">
              {model.name || model.id}
            </CardTitle>
            <p className="mt-0.5 truncate font-mono text-[11px] text-muted-foreground">{model.id}</p>
          </div>
          <Button
            variant="ghost"
            size="icon"
            className="size-8 shrink-0 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100"
            aria-label="Copy model id"
            onClick={async () => {
              await navigator.clipboard.writeText(model.id)
              setCopied(true)
              toast.success("Model id copied")
              window.setTimeout(() => setCopied(false), 1500)
            }}
          >
            {copied ? <Check className="text-emerald-400" /> : <Copy />}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="line-clamp-2 min-h-8 text-xs leading-relaxed text-muted-foreground">
          {model.description || "No description provided by the upstream catalog."}
        </p>
        <div className="flex flex-wrap items-center gap-1.5">
          <Badge variant="secondary" className="font-normal text-[10px] text-muted-foreground">
            {contextLabel(model.max_context_length)}
          </Badge>
          {capabilities.map(([key]) => {
            const meta = capabilityMeta[key]
            const Icon = meta.icon
            return (
              <Badge
                key={key}
                variant="outline"
                className="gap-1 border-border/70 bg-background/30 font-normal text-[10px] text-foreground/80"
              >
                <Icon className="size-3" />
                {meta.label}
              </Badge>
            )
          })}
        </div>
      </CardContent>
    </Card>
  )
}

const filters = [
  { key: "chat", label: "Chat" },
  { key: "tools", label: "Tools" },
  { key: "vision", label: "Vision" },
  { key: "reasoning", label: "Reasoning" },
  { key: "all", label: "All" },
] as const

type Filter = (typeof filters)[number]["key"]

function matchesFilter(model: Model, filter: Filter): boolean {
  if (filter === "all") return true
  const capabilities = model.capabilities ?? {}
  if (filter === "tools") return capabilities.function_calling === true
  if (filter === "chat") return capabilities.completion_chat === true
  if (filter === "vision") return capabilities.vision === true
  return capabilities.reasoning === true
}

export function ModelsPanel({ enabled }: { enabled: boolean }) {
  const [search, setSearch] = useState("")
  const [filter, setFilter] = useState<Filter>("chat")
  const [showAll, setShowAll] = useState(false)
  const models = useQuery({
    queryKey: ["models"],
    queryFn: api.models,
    enabled,
    refetchInterval: 60_000,
  })

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase()
    const list = (models.data?.data ?? []).filter((model) => matchesFilter(model, filter))
    if (!query) return list
    return list.filter((model) =>
      [model.id, model.name, model.description].some((value) =>
        value?.toLowerCase().includes(query),
      ),
    )
  }, [models.data, search, filter])

  const visible = showAll ? filtered : filtered.slice(0, 9)

  return (
    <Card className="glass animate-rise overflow-hidden">
      <CardHeader className="flex flex-col gap-3 border-b border-border/50 pb-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <CardTitle className="flex items-center gap-2 text-base">
            <Cpu className="size-4 text-accent" />
            Model catalog
          </CardTitle>
          <p className="mt-1 text-xs text-muted-foreground">
            Live from <span className="font-mono">/v1/models</span>
            {models.data ? ` · ${models.data.data.length} available` : ""}
          </p>
        </div>
        <div className="relative w-full sm:w-72">
          <Search className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search models…"
            className="h-9 bg-background/40 pl-9"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
        </div>
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          {filters.map((option) => (
            <Button
              key={option.key}
              type="button"
              variant={filter === option.key ? "secondary" : "ghost"}
              size="sm"
              className={
                filter === option.key
                  ? "h-7 border border-border/70 bg-secondary/80 px-2.5 text-[11px]"
                  : "h-7 px-2.5 text-[11px] text-muted-foreground"
              }
              onClick={() => {
                setFilter(option.key)
                setShowAll(false)
              }}
            >
              {option.label}
            </Button>
          ))}
        </div>
      </CardHeader>
      <CardContent className="pt-4">
        {!enabled ? (
          <div className="flex flex-col items-center gap-2 py-14 text-center">
            <TriangleAlert className="size-6 text-amber-400" />
            <p className="text-sm font-medium">Add an API key to load models</p>
            <p className="max-w-sm text-xs text-muted-foreground">
              The catalog is fetched from the Mistral API using the key stored by this proxy.
            </p>
          </div>
        ) : models.isPending ? (
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            {Array.from({ length: 6 }).map((_, index) => (
              <Skeleton key={index} className="h-36 rounded-xl" />
            ))}
          </div>
        ) : models.isError ? (
          <div className="flex flex-col items-center gap-2 py-14 text-center">
            <TriangleAlert className="size-6 text-destructive" />
            <p className="text-sm font-medium">Could not load the catalog</p>
            <p className="max-w-md text-xs text-muted-foreground">{(models.error as Error).message}</p>
            <Button variant="outline" size="sm" onClick={() => models.refetch()}>
              Retry
            </Button>
          </div>
        ) : filtered.length === 0 ? (
          <p className="py-14 text-center text-sm text-muted-foreground">No models match “{search}”.</p>
        ) : (
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            {visible.map((model) => (
              <ModelCard key={`${model.id}-${model.name ?? ""}`} model={model} />
            ))}
          </div>
        )}
        {filtered.length > 9 ? (
          <div className="mt-4 flex justify-center">
            <Button variant="outline" size="sm" onClick={() => setShowAll((current) => !current)}>
              {showAll ? "Show fewer" : `Show all ${filtered.length} models`}
            </Button>
          </div>
        ) : null}
        {models.data ? (
          <div className="mt-4 flex items-center justify-between text-[11px] text-muted-foreground">
            <span>
              Showing {visible.length} of {filtered.length} matching · {models.data.data.length} total
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
        ) : null}
      </CardContent>
    </Card>
  )
}
