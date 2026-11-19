import type { LucideIcon } from "lucide-react"

import { cn } from "cn"

type Props = {
  label: string
  value: string
  detail: string
  icon: LucideIcon
  tone?: "violet" | "cyan" | "emerald" | "amber"
  delay?: number
}

const tones = {
  violet: "bg-primary/12 text-primary ring-primary/25",
  cyan: "bg-accent/12 text-accent ring-accent/25",
  emerald: "bg-emerald-400/10 text-emerald-400 ring-emerald-400/25",
  amber: "bg-amber-400/10 text-amber-400 ring-amber-400/25",
}

const glows = {
  violet: "bg-primary/15",
  cyan: "bg-accent/15",
  emerald: "bg-emerald-400/12",
  amber: "bg-amber-400/12",
}

const accents = {
  violet: "via-primary/60",
  cyan: "via-accent/60",
  emerald: "via-emerald-400/50",
  amber: "via-amber-400/50",
}

export function StatCard({ label, value, detail, icon: Icon, tone = "violet", delay = 0 }: Props) {
  return (
    <div
      className="glass animate-rise group relative overflow-hidden rounded-2xl p-4 transition-all duration-300 hover:-translate-y-0.5 hover:shadow-[0_0_36px_-12px_var(--ring)]"
      style={{ animationDelay: `${delay}ms` }}
    >
      <div className={cn("absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent to-transparent", accents[tone])} />
      <div className={cn("pointer-events-none absolute -top-16 -right-10 size-32 rounded-full opacity-30 blur-2xl transition-opacity duration-300 group-hover:opacity-80", glows[tone])} />
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-[11px] font-medium tracking-[0.14em] text-muted-foreground uppercase">{label}</p>
          <p className="mt-2 text-2xl font-semibold tracking-tight text-foreground tabular-nums">{value}</p>
        </div>
        <span className={cn("flex size-9 shrink-0 items-center justify-center rounded-xl ring-1", tones[tone])}>
          <Icon className="size-4" />
        </span>
      </div>
      <p className="mt-2 truncate text-[11px] text-muted-foreground">{detail}</p>
    </div>
  )
}
