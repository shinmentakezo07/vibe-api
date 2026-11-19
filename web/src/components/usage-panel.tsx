import { useMemo } from "react"
import { Bar, BarChart, CartesianGrid, XAxis } from "recharts"
import { Activity, Gauge, Timer, Waves } from "lucide-react"

import { compactNumber, type UsageSnapshot } from "@/lib/api"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart"
import { Skeleton } from "@/components/ui/skeleton"

const throughputConfig = {
  output: { label: "Output tokens", color: "var(--chart-2)" },
} satisfies ChartConfig

type ThroughputBucket = { time: string; label: string; output: number; total: number }

function buildThroughput(snapshot?: UsageSnapshot): ThroughputBucket[] {
  const buckets = new Map<number, ThroughputBucket>()
  const now = Date.now()
  const minute = 60_000
  for (let index = 19; index >= 0; index--) {
    const stamp = Math.floor((now - index * minute) / minute) * minute
    buckets.set(stamp, {
      time: new Date(stamp).toISOString(),
      label: new Date(stamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
      output: 0,
      total: 0,
    })
  }
  for (const record of snapshot?.recent ?? []) {
    const stamp = Math.floor(new Date(record.time).getTime() / minute) * minute
    const bucket = buckets.get(stamp)
    if (bucket) {
      bucket.output += record.completionTokens
      bucket.total += record.totalTokens
    }
  }
  return [...buckets.values()]
}

function PulseBar({ height, index }: { height: number; index: number }) {
  return (
    <div className="flex h-full flex-1 items-end">
      <div
        className="w-full rounded-sm bg-gradient-to-t from-primary/30 to-primary transition-all duration-500"
        style={{ height: `${Math.max(4, height)}%`, opacity: 0.35 + (index / 60) * 0.65 }}
      />
    </div>
  )
}

function WindowGauge({
  label,
  value,
  max,
  display,
  detail,
  color,
}: {
  label: string
  value: number
  max: number
  display: string
  detail: string
  color: string
}) {
  const percent = Math.min(100, Math.max(0, (value / max) * 100))
  const circumference = 2 * Math.PI * 42
  return (
    <div className="flex flex-col items-center gap-2 text-center">
      <div className="relative size-24">
        <svg viewBox="0 0 100 100" className="size-full -rotate-90">
          <circle cx="50" cy="50" r="42" fill="none" stroke="var(--border)" strokeWidth="7" />
          <circle
            cx="50"
            cy="50"
            r="42"
            fill="none"
            stroke={color}
            strokeWidth="7"
            strokeLinecap="round"
            strokeDasharray={`${(percent / 100) * circumference} ${circumference}`}
            className="transition-all duration-700"
          />
        </svg>
        <div className="absolute inset-0 flex flex-col items-center justify-center">
          <span className="font-mono text-lg font-semibold leading-none">{display}</span>
          <span className="mt-1 text-[9px] tracking-wider text-muted-foreground uppercase">{label}</span>
        </div>
      </div>
      <p className="text-[10px] text-muted-foreground">{detail}</p>
    </div>
  )
}

function duration(ms: number) {
  if (ms < 1000) return `${Math.max(1, Math.round(ms))} ms`
  if (ms < 10_000) return `${(ms / 1000).toFixed(1)} s`
  return `${Math.round(ms / 1000)} s`
}

function percentile(sorted: number[], fraction: number) {
  if (sorted.length === 0) return 0
  return sorted[Math.min(sorted.length - 1, Math.max(0, Math.ceil(sorted.length * fraction) - 1))]
}

export function UsagePanel({ snapshot, pending }: { snapshot?: UsageSnapshot; pending: boolean }) {
  const throughput = useMemo(() => buildThroughput(snapshot), [snapshot])

  const pulse = useMemo(() => {
    const now = Date.now()
    const seconds = new Map<number, number>()
    for (let index = 59; index >= 0; index--) {
      seconds.set(Math.floor((now - index * 1000) / 1000) * 1000, 0)
    }
    for (const record of snapshot?.recent ?? []) {
      const stamp = Math.floor(new Date(record.time).getTime() / 1000) * 1000
      if (seconds.has(stamp)) seconds.set(stamp, (seconds.get(stamp) ?? 0) + record.totalTokens)
    }
    const values = [...seconds.values()]
    const peak = Math.max(1, ...values)
    return values.map((value) => (value / peak) * 100)
  }, [snapshot])

  const rates = useMemo(() => {
    const now = Date.now()
    const records = (snapshot?.recent ?? []).filter(
      (record) => now - new Date(record.time).getTime() <= 60_000,
    )
    const total = records.reduce((sum, record) => sum + record.totalTokens, 0)
    const output = records.reduce((sum, record) => sum + record.completionTokens, 0)
    const prompt = records.reduce((sum, record) => sum + record.promptTokens, 0)
    const cached = records.reduce((sum, record) => sum + record.cachedInputTokens, 0)
    const reasoning = records.reduce((sum, record) => sum + record.reasoningTokens, 0)
    return {
      tpm: total,
      tps: output / 60,
      requests: records.length,
      cacheHit: prompt > 0 ? (cached / prompt) * 100 : 0,
      reasoningShare: output > 0 ? (reasoning / output) * 100 : 0,
    }
  }, [snapshot])

  const peaks = useMemo(() => {
    const peakTotal = Math.max(1, ...throughput.map((bucket) => bucket.total))
    const peakOutput = Math.max(1, ...throughput.map((bucket) => bucket.output))
    return {
      tpm: Math.max(1000, peakTotal),
      tps: Math.max(10, peakOutput / 60),
    }
  }, [throughput])

  const totals = snapshot?.totals

  const latencyRecords = useMemo(
    () => (snapshot?.recent ?? []).filter((record) => record.latencyMs > 0),
    [snapshot],
  )
  const latencyBuckets = useMemo(() => {
    const edges = [250, 500, 1000, 2000, 5000, Number.POSITIVE_INFINITY]
    const labels = ["<250 ms", "250–500 ms", "0.5–1 s", "1–2 s", "2–5 s", "5 s+"]
    const counts = labels.map(() => 0)
    for (const record of latencyRecords) {
      const index = edges.findIndex((edge) => record.latencyMs < edge)
      counts[index === -1 ? labels.length - 1 : index] += 1
    }
    const max = Math.max(1, ...counts)
    return labels.map((label, index) => ({
      label,
      count: counts[index],
      percent: (counts[index] / max) * 100,
    }))
  }, [latencyRecords])

  const sortedLatency = useMemo(
    () => latencyRecords.map((record) => record.latencyMs).sort((a, b) => a - b),
    [latencyRecords],
  )

  return (
    <div className="space-y-5">
      <Card className="glass animate-rise overflow-hidden">
        <CardHeader className="flex flex-row items-center justify-between border-b border-border/50 pb-4">
          <div>
            <CardTitle className="flex items-center gap-2 text-base">
              <Activity className="size-4 text-primary" />
              Live pulse
            </CardTitle>
            <p className="mt-1 text-xs text-muted-foreground">
              Rolling 60-second window · {rates.requests} {rates.requests === 1 ? "request" : "requests"}
            </p>
          </div>
          <span className="inline-flex items-center gap-2 rounded-full border border-border/60 bg-background/40 px-2.5 py-1 text-[10px] font-medium tracking-wide text-muted-foreground uppercase">
            <span className="live-dot size-1.5 rounded-full bg-emerald-400" />
            Live
          </span>
        </CardHeader>
        <CardContent className="space-y-5 pt-5">
          <div className="grid grid-cols-2 gap-5 sm:grid-cols-4">
            <WindowGauge
              label="TPM"
              value={rates.tpm}
              max={peaks.tpm}
              display={compactNumber(rates.tpm)}
              detail="tokens / minute"
              color="var(--chart-1)"
            />
            <WindowGauge
              label="TPS"
              value={rates.tps}
              max={peaks.tps}
              display={rates.tps >= 10 ? compactNumber(Math.round(rates.tps)) : rates.tps.toFixed(1)}
              detail="output tokens / second"
              color="var(--chart-2)"
            />
            <WindowGauge
              label="Cache hit"
              value={rates.cacheHit}
              max={100}
              display={`${Math.round(rates.cacheHit)}%`}
              detail="prompt served from cache"
              color="var(--chart-3)"
            />
            <WindowGauge
              label="Reasoning"
              value={rates.reasoningShare}
              max={100}
              display={`${Math.round(rates.reasoningShare)}%`}
              detail="share of output tokens"
              color="var(--chart-4)"
            />
          </div>

          <div>
            <div className="mb-2 flex items-center justify-between text-[10px] tracking-wider text-muted-foreground uppercase">
              <span className="inline-flex items-center gap-1.5">
                <Waves className="size-3" />
                60-second pulse
              </span>
              <span>tokens / second</span>
            </div>
            <div className="flex h-14 items-end gap-[2px]">
              {pulse.map((height, index) => (
                <PulseBar key={index} height={height} index={index} />
              ))}
            </div>
          </div>
        </CardContent>
      </Card>

      <div className="grid gap-5 xl:grid-cols-[1.45fr_1fr]">
        <Card className="glass animate-rise overflow-hidden">
          <CardHeader className="flex flex-row items-center justify-between border-b border-border/50 pb-4">
            <div>
              <CardTitle className="flex items-center gap-2 text-base">
                <Gauge className="size-4 text-emerald-400" />
                Output throughput
              </CardTitle>
              <p className="mt-1 text-xs text-muted-foreground">Completion tokens per minute, last 20 minutes</p>
            </div>
            {totals ? (
              <span className="rounded-md border border-border/60 bg-background/30 px-2 py-1 font-mono text-[10px] text-muted-foreground">
                {compactNumber(totals.completionTokens)} out
              </span>
            ) : null}
          </CardHeader>
          <CardContent className="pt-5">
            {pending && !snapshot ? (
              <Skeleton className="h-[220px] rounded-xl" />
            ) : (
              <ChartContainer config={throughputConfig} className="h-[220px] w-full">
                <BarChart data={throughput} margin={{ left: 0, right: 8, top: 8, bottom: 0 }}>
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
                  <ChartTooltip cursor={false} content={<ChartTooltipContent indicator="line" />} />
                  <Bar
                    dataKey="output"
                    fill="var(--color-output)"
                    radius={[4, 4, 0, 0]}
                    maxBarSize={18}
                  />
                </BarChart>
              </ChartContainer>
            )}
            {!snapshot?.totals.requests && !pending ? (
              <p className="-mt-2 text-center text-xs text-muted-foreground">
                No output yet. Throughput appears after the first completion.
              </p>
            ) : null}
          </CardContent>
        </Card>

        <Card className="glass animate-rise overflow-hidden">
          <CardHeader className="flex flex-row items-center justify-between border-b border-border/50 pb-4">
            <div>
              <CardTitle className="flex items-center gap-2 text-base">
                <Timer className="size-4 text-sky-400" />
                Latency distribution
              </CardTitle>
              <p className="mt-1 text-xs text-muted-foreground">End-to-end proxy response time</p>
            </div>
            {sortedLatency.length > 0 ? (
              <div className="flex gap-1.5">
                <span className="rounded-md border border-border/60 bg-background/30 px-1.5 py-1 font-mono text-[9px] text-muted-foreground">
                  p50 {duration(percentile(sortedLatency, 0.5))}
                </span>
                <span className="rounded-md border border-border/60 bg-background/30 px-1.5 py-1 font-mono text-[9px] text-muted-foreground">
                  p95 {duration(percentile(sortedLatency, 0.95))}
                </span>
              </div>
            ) : null}
          </CardHeader>
          <CardContent className="pt-5">
            {latencyRecords.length === 0 ? (
              <div className="flex h-[180px] flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-border/70 px-4 text-center">
                <Timer className="size-5 text-muted-foreground/70" />
                <p className="text-xs text-muted-foreground">
                  Latency appears after the next completion is proxied.
                </p>
              </div>
            ) : (
              <div className="space-y-3">
                {latencyBuckets.map((bucket) => (
                  <div key={bucket.label} className="space-y-1">
                    <div className="flex items-baseline justify-between gap-2 text-[10px]">
                      <span className="font-mono text-muted-foreground">{bucket.label}</span>
                      <span className="text-foreground/75">{bucket.count}</span>
                    </div>
                    <div className="h-2 overflow-hidden rounded-full bg-secondary/70">
                      <div
                        className="h-full rounded-full bg-gradient-to-r from-sky-500/70 to-emerald-400 transition-all duration-500"
                        style={{ width: `${bucket.percent}%` }}
                      />
                    </div>
                  </div>
                ))}
                <p className="pt-1 text-[10px] text-muted-foreground">
                  {latencyRecords.length} timed {latencyRecords.length === 1 ? "request" : "requests"} · max{" "}
                  {duration(sortedLatency[sortedLatency.length - 1])}
                </p>
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
