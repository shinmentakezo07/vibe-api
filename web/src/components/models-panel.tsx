import { useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { ArrowUpRight, Cpu, Search, TriangleAlert } from "lucide-react"

import { api, type Model } from "@/lib/api"
import { ModelCard } from "@/components/model-card"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"

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

export function ModelsPanel({ enabled, onViewAll }: { enabled: boolean; onViewAll: () => void }) {
  const [search, setSearch] = useState("")
  const [filter, setFilter] = useState<Filter>("chat")
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

  const visible = filtered.slice(0, 9)

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
              onClick={() => setFilter(option.key)}
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
            <Button variant="outline" size="sm" onClick={onViewAll}>
              Browse all {filtered.length} matching models
              <ArrowUpRight />
            </Button>
          </div>
        ) : null}
        {models.data ? (
          <div className="mt-4 flex items-center justify-between text-[11px] text-muted-foreground">
            <span>
              Showing {visible.length} of {filtered.length} matching · {models.data.data.length} total
            </span>
            <div className="flex items-center gap-3">
              <button
                type="button"
                className="inline-flex items-center gap-1 font-medium text-primary transition-colors hover:text-primary/80"
                onClick={onViewAll}
              >
                Full catalog <ArrowUpRight className="size-3" />
              </button>
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
        ) : null}
      </CardContent>
    </Card>
  )
}
