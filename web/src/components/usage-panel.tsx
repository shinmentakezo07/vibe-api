import { useMemo } from "react"
import { Area, AreaChart, CartesianGrid, XAxis } from "recharts"
import { Activity, BarChart3 } from "lucide-react"

import { compactNumber, type UsageSnapshot } from "@/lib/api"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart"
import { Skeleton } from "@/components/ui/skeleton"

const chartConfig = {
  prompt: { label: "Input", color: "var(--chart-1)" },
  completion: { label: "Output", color: "var(--chart-2)" },
} satisfies ChartConfig

type Bucket = { time: string; label: string; prompt: number; completion: number }

function buildTimeline(snapshot?: UsageSnapshot): Bucket[] {
  const buckets = new Map<number, Bucket>()
  const now = Date.now()
  const minute = 60_000
  for (let index = 19; index >= 0; index--) {
    const stamp = Math.floor((now - index * minute) / minute) * minute
    buckets.set(stamp, {
      time: new Date(stamp).toISOString(),
      label: new Date(stamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
      prompt: 0,
      completion: 0,
    })
  }
  for (const record of snapshot?.recent ?? []) {
    const stamp = Math.floor(new Date(record.time).getTime() / minute) * minute
    const bucket = buckets.get(stamp)
    if (bucket) {
      bucket.prompt += record.promptTokens
      bucket.completion += record.completionTokens
    }
  }
  return [...buckets.values()]
}

export function UsagePanel({ snapshot, pending }: { snapshot?: UsageSnapshot; pending: boolean }) {
  const timeline = useMemo(() => buildTimeline(snapshot), [snapshot])
  const models = useMemo(
    () => [...(snapshot?.byModel ?? [])].sort((a, b) => b.totalTokens - a.totalTokens).slice(0, 6),
    [snapshot],
  )
  const maxModelTokens = Math.max(1, ...models.map((model) => model.totalTokens))
  const hasTraffic = (snapshot?.totals.requests ?? 0) > 0

  return (
    <Card className="glass animate-rise overflow-hidden">
      <CardHeader className="flex flex-row items-center justify-between border-b border-border/50 pb-4">
        <div>
          <CardTitle className="flex items-center gap-2 text-base">
            <Activity className="size-4 text-primary" />
            Token flow
          </CardTitle>
          <p className="mt-1 text-xs text-muted-foreground">Last 20 minutes, live from proxied traffic</p>
        </div>
        <span className="inline-flex items-center gap-2 rounded-full border border-border/60 bg-background/40 px-2.5 py-1 text-[10px] font-medium tracking-wide text-muted-foreground uppercase">
          <span className="live-dot size-1.5 rounded-full bg-emerald-400" />
          Live
        </span>
      </CardHeader>
      <CardContent className="grid gap-6 pt-5 lg:grid-cols-[1.55fr_1fr]">
        <div className="min-w-0">
          {pending && !snapshot ? (
            <Skeleton className="h-[240px] rounded-xl" />
          ) : (
            <ChartContainer config={chartConfig} className="h-[240px] w-full">
              <AreaChart data={timeline} margin={{ left: 0, right: 8, top: 8, bottom: 0 }}>
                <defs>
                  <linearGradient id="fillPrompt" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="var(--color-prompt)" stopOpacity={0.5} />
                    <stop offset="100%" stopColor="var(--color-prompt)" stopOpacity={0.02} />
                  </linearGradient>
                  <linearGradient id="fillCompletion" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="var(--color-completion)" stopOpacity={0.4} />
                    <stop offset="100%" stopColor="var(--color-completion)" stopOpacity={0.02} />
                  </linearGradient>
                </defs>
                <CartesianGrid vertical={false} strokeDasharray="4 6" stroke="var(--border)" />
                <XAxis
                  dataKey="label"
                  tickLine={false}
                  axisLine={false}
                  tickMargin={8}
                  interval="preserveStartEnd"
                  minTickGap={28}
                  tick={{ fill: "var(--muted-foreground)", fontSize: 10 }}
                />
                <ChartTooltip cursor={false} content={<ChartTooltipContent indicator="dot" />} />
                <Area
                  dataKey="prompt"
                  type="monotone"
                  fill="url(#fillPrompt)"
                  stroke="var(--color-prompt)"
                  strokeWidth={2}
                  stackId="tokens"
                />
                <Area
                  dataKey="completion"
                  type="monotone"
                  fill="url(#fillCompletion)"
                  stroke="var(--color-completion)"
                  strokeWidth={2}
                  stackId="tokens"
                />
              </AreaChart>
            </ChartContainer>
          )}
          {!hasTraffic && !pending ? (
            <p className="-mt-2 text-center text-xs text-muted-foreground">
              No requests yet. Usage appears here as soon as a completion is proxied.
            </p>
          ) : null}
        </div>

        <div className="space-y-3">
          <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
            <BarChart3 className="size-3.5" />
            By model
          </div>
          {models.length === 0 ? (
            <div className="rounded-xl border border-dashed border-border/70 px-4 py-8 text-center text-xs text-muted-foreground">
              Model usage will appear after the first completion.
            </div>
          ) : (
            <div className="space-y-3">
              {models.map((model, index) => (
                <div key={model.model} className="space-y-1.5">
                  <div className="flex items-baseline justify-between gap-3">
                    <span className="truncate font-mono text-[11px] text-foreground/85">{model.model}</span>
                    <span className="shrink-0 text-[10px] text-muted-foreground">
                      {compactNumber(model.totalTokens)} tok
                    </span>
                  </div>
                  <div className="h-1.5 overflow-hidden rounded-full bg-secondary/70">
                    <div
                      className="h-full rounded-full transition-all duration-500"
                      style={{
                        width: `${Math.max(3, (model.totalTokens / maxModelTokens) * 100)}%`,
                        background: `var(--chart-${(index % 5) + 1})`,
                      }}
                    />
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
