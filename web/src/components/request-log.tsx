import { ArrowDownRight, ArrowUpRight, Clock3, Waves } from "lucide-react"

import { compactNumber, formatNumber, timeAgo, type UsageSnapshot } from "@/lib/api"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

function latency(ms: number) {
  if (ms <= 0) return "—"
  if (ms < 1000) return `${Math.round(ms)} ms`
  if (ms < 10_000) return `${(ms / 1000).toFixed(1)} s`
  return `${Math.round(ms / 1000)} s`
}

export function RequestLog({ snapshot }: { snapshot?: UsageSnapshot }) {
  const records = (snapshot?.recent ?? []).slice(0, 20)

  return (
    <Card className="glass animate-rise overflow-hidden">
      <CardHeader className="flex flex-row items-center justify-between border-b border-border/50 pb-4">
        <div>
          <CardTitle className="flex items-center gap-2 text-base">
            <Clock3 className="size-4 text-emerald-400" />
            Request log
          </CardTitle>
          <p className="mt-1 text-xs text-muted-foreground">
            Newest completions with token, cache, reasoning, and latency accounting
          </p>
        </div>
        {snapshot ? (
          <Badge variant="outline" className="border-border/70 bg-background/30 font-mono text-[10px]">
            {formatNumber(snapshot.totals.requests)} total
          </Badge>
        ) : null}
      </CardHeader>
      <CardContent className="px-0 pb-1">
        {records.length === 0 ? (
          <div className="flex flex-col items-center gap-2 px-6 py-12 text-center">
            <Waves className="size-6 text-muted-foreground/70" />
            <p className="text-sm font-medium">Waiting for traffic</p>
            <p className="max-w-sm text-xs text-muted-foreground">
              Point Vibe, the OpenAI SDK, or the Anthropic SDK at this proxy and requests will show up here.
            </p>
          </div>
        ) : (
          <div className="max-h-[32rem] overflow-auto">
            <Table>
              <TableHeader className="sticky top-0 z-10 bg-card/95 backdrop-blur">
                <TableRow className="border-border/50 hover:bg-transparent">
                  <TableHead className="pl-5 text-[10px] tracking-wider uppercase">Model</TableHead>
                  <TableHead className="text-[10px] tracking-wider uppercase">Endpoint</TableHead>
                  <TableHead className="text-right text-[10px] tracking-wider uppercase">Input</TableHead>
                  <TableHead className="text-right text-[10px] tracking-wider uppercase">Output</TableHead>
                  <TableHead className="text-right text-[10px] tracking-wider uppercase">Cached</TableHead>
                  <TableHead className="text-right text-[10px] tracking-wider uppercase">Reason</TableHead>
                  <TableHead className="text-right text-[10px] tracking-wider uppercase">Total</TableHead>
                  <TableHead className="text-right text-[10px] tracking-wider uppercase">Latency</TableHead>
                  <TableHead className="pr-5 text-right text-[10px] tracking-wider uppercase">When</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {records.map((record) => (
                  <TableRow key={record.id} className="border-border/40">
                    <TableCell className="max-w-[13rem] pl-5">
                      <div className="flex items-center gap-2">
                        <span className="truncate font-mono text-xs text-foreground/90">{record.model}</span>
                        {record.stream ? (
                          <span className="rounded bg-primary/10 px-1 py-0.5 text-[9px] text-primary">SSE</span>
                        ) : null}
                      </div>
                    </TableCell>
                    <TableCell>
                      <span className="rounded-md border border-border/60 bg-background/30 px-1.5 py-0.5 font-mono text-[10px] whitespace-nowrap text-muted-foreground">
                        {record.route}
                      </span>
                    </TableCell>
                    <TableCell className="text-right">
                      <span className="inline-flex items-center gap-0.5 font-mono text-xs text-muted-foreground">
                        <ArrowDownRight className="size-3 text-primary/80" />
                        {compactNumber(record.promptTokens)}
                      </span>
                    </TableCell>
                    <TableCell className="text-right">
                      <span className="inline-flex items-center gap-0.5 font-mono text-xs text-muted-foreground">
                        <ArrowUpRight className="size-3 text-accent/80" />
                        {compactNumber(record.completionTokens)}
                      </span>
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs text-muted-foreground">
                      {record.cachedInputTokens > 0 ? compactNumber(record.cachedInputTokens) : "—"}
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs text-muted-foreground">
                      {record.reasoningTokens > 0 ? compactNumber(record.reasoningTokens) : "—"}
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs text-foreground/85">
                      {compactNumber(record.totalTokens)}
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs text-foreground/75">
                      {latency(record.latencyMs)}
                    </TableCell>
                    <TableCell className="pr-5 text-right text-[11px] whitespace-nowrap text-muted-foreground">
                      {timeAgo(record.time)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
