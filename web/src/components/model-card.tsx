import { useState } from "react"
import { BrainCircuit, Check, Copy, Cpu, Eye, FunctionSquare, Sparkles } from "lucide-react"
import { toast } from "sonner"

import type { Model } from "@/lib/api"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

export const capabilityMeta: Record<string, { label: string; icon: typeof Cpu }> = {
  completion_chat: { label: "Chat", icon: Sparkles },
  function_calling: { label: "Tools", icon: FunctionSquare },
  vision: { label: "Vision", icon: Eye },
  reasoning: { label: "Reasoning", icon: BrainCircuit },
  completion_fim: { label: "FIM", icon: Cpu },
  fine_tuning: { label: "Fine-tune", icon: Cpu },
  ocr: { label: "OCR", icon: Cpu },
}

export function contextLabel(value?: number) {
  if (!value) return "—"
  if (value >= 1000) return `${Math.round(value / 1000)}K context`
  return `${value} context`
}

export function ModelCard({ model, allCapabilities = false }: { model: Model; allCapabilities?: boolean }) {
  const [copied, setCopied] = useState(false)
  const capabilities = Object.entries(model.capabilities ?? {}).filter(
    ([key, enabled]) => enabled && capabilityMeta[key],
  )
  const shown = allCapabilities ? capabilities : capabilities.slice(0, 4)

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
            aria-label={`Copy ${model.id}`}
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
          {shown.map(([key]) => {
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
