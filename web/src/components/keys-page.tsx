import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  AlertTriangle,
  Database,
  Eye,
  EyeOff,
  KeyRound,
  Loader2,
  Plus,
  RefreshCw,
  ShieldCheck,
  Trash2,
} from "lucide-react"
import { toast } from "sonner"

import { api, compactNumber, timeAgo, type KeyList } from "@/lib/api"
import { cn } from "cn"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

type Props = {
  onBack: () => void
}

function Toggle({
  checked,
  onChange,
  disabled,
  label,
}: {
  checked: boolean
  onChange: (checked: boolean) => void
  disabled?: boolean
  label: string
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={cn(
        "relative h-6 w-11 shrink-0 rounded-full border transition-colors disabled:opacity-50",
        checked
          ? "border-primary/40 bg-primary/80 shadow-[0_0_16px_-4px_var(--primary)]"
          : "border-border bg-secondary",
      )}
    >
      <span
        className={cn(
          "absolute top-0.5 left-0.5 size-4.5 rounded-full bg-white shadow-sm transition-transform",
          checked ? "translate-x-5" : "translate-x-0",
        )}
      />
    </button>
  )
}

function sourceLabel(source: KeyList["keys"][number]["source"]) {
  if (source === "environment") return "ENV"
  if (source === "memory") return "MEMORY"
  return "POSTGRES"
}

export function KeysPage({ onBack }: Props) {
  const queryClient = useQueryClient()
  const [label, setLabel] = useState("")
  const [secret, setSecret] = useState("")
  const [visible, setVisible] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [openErrors, setOpenErrors] = useState<string | null>(null)
  const [pendingDelete, setPendingDelete] = useState<string | null>(null)

  const keys = useQuery({
    queryKey: ["keys"],
    queryFn: api.keys,
    refetchInterval: 15_000,
    retry: 1,
  })

  const refreshDerived = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["status"] }),
      queryClient.invalidateQueries({ queryKey: ["models"] }),
    ])
  }

  const apply = (next: KeyList) => {
    queryClient.setQueryData(["keys"], next)
  }

  const add = useMutation({
    mutationFn: ({ label, secret }: { label: string; secret: string }) => api.addKey(label, secret),
    onSuccess: async (next) => {
      apply(next)
      setLabel("")
      setSecret("")
      setVisible(false)
      setError(null)
      await refreshDerived()
      toast.success("API key validated and added")
    },
    onError: (err: Error) => setError(err.message),
  })

  const update = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) => api.updateKey(id, { enabled }),
    onSuccess: async (next) => {
      apply(next)
      await refreshDerived()
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const remove = useMutation({
    mutationFn: (id: string) => api.deleteKey(id),
    onSuccess: async (next) => {
      apply(next)
      setPendingDelete(null)
      await refreshDerived()
      toast.success("API key removed")
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const roundRobin = useMutation({
    mutationFn: (enabled: boolean) => api.setRoundRobin(enabled),
    onSuccess: async (next) => {
      apply(next)
      toast.success(next.roundRobin ? "Round-robin enabled" : "Round-robin disabled")
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const data = keys.data
  const enabled = data?.keys.filter((key) => key.enabled).length ?? 0
  const total = data?.keys.length ?? 0
  const busyToggle = update.isPending || remove.isPending

  return (
    <>
      <section className="animate-rise flex flex-col justify-between gap-4 pt-2 pb-1 lg:flex-row lg:items-end">
        <div>
          <div className="inline-flex items-center gap-2 rounded-full border border-primary/25 bg-primary/10 px-3 py-1 text-[10px] font-medium tracking-[0.18em] text-primary uppercase">
            <KeyRound className="size-3" />
            Credential pool
          </div>
          <h1 className="text-gradient mt-4 text-4xl font-semibold tracking-tight sm:text-5xl">API keys</h1>
          <p className="mt-3 max-w-xl text-sm leading-relaxed text-muted-foreground">
            Add multiple Mistral keys, disable one without deleting it, and rotate traffic across every
            enabled credential.
          </p>
        </div>
        <div className="flex items-center gap-3">
          <div className="glass rounded-xl px-4 py-2.5 text-xs">
            <p className="text-[10px] tracking-wider text-muted-foreground uppercase">Enabled</p>
            <p className="mt-0.5 font-mono text-sm text-foreground">
              {data ? `${enabled} / ${total}` : "—"}
            </p>
          </div>
          <Button variant="outline" size="sm" className="h-9 border-border/70 bg-background/30" onClick={onBack}>
            Overview
          </Button>
        </div>
      </section>

      {keys.isError ? (
        <div className="rounded-xl border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-red-300">
          Could not load API keys: {(keys.error as Error).message}
        </div>
      ) : null}

      <div className="grid gap-5 lg:grid-cols-[minmax(0,380px)_minmax(0,1fr)]">
        <div className="space-y-5">
          <div className="glass rounded-2xl p-5">
            <div className="flex items-center gap-2">
              <Plus className="size-4 text-primary" />
              <h2 className="text-sm font-semibold">Add API key</h2>
            </div>
            <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
              Validated with a models request before it joins the pool.
            </p>

            <form
              className="mt-4 space-y-3"
              onSubmit={(event) => {
                event.preventDefault()
                setError(null)
                add.mutate({ label: label.trim(), secret: secret.trim() })
              }}
            >
              <div className="space-y-1.5">
                <Label htmlFor="key-label">Label</Label>
                <Input
                  id="key-label"
                  placeholder="Production, Team A…"
                  maxLength={100}
                  value={label}
                  onChange={(event) => setLabel(event.target.value)}
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="new-api-key">API key</Label>
                <div className="relative">
                  <Input
                    id="new-api-key"
                    autoComplete="off"
                    spellCheck={false}
                    type={visible ? "text" : "password"}
                    placeholder="••••••••••••••••••••••••"
                    className="h-11 pr-11 font-mono"
                    value={secret}
                    onChange={(event) => setSecret(event.target.value)}
                  />
                  <button
                    type="button"
                    aria-label={visible ? "Hide key" : "Show key"}
                    onClick={() => setVisible((current) => !current)}
                    className="absolute top-1/2 right-3 -translate-y-1/2 text-muted-foreground transition-colors hover:text-foreground"
                  >
                    {visible ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                  </button>
                </div>
              </div>

              {error ? (
                <p className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-red-300">
                  {error}
                </p>
              ) : null}

              <Button type="submit" className="w-full" disabled={add.isPending || secret.trim().length === 0}>
                {add.isPending ? <Loader2 className="animate-spin" /> : <ShieldCheck />}
                Validate &amp; add
              </Button>
            </form>

            <p className="mt-3 flex items-start gap-1.5 text-[11px] leading-relaxed text-muted-foreground">
              <Database className="mt-0.5 size-3 shrink-0" />
              {data?.storage === "postgres"
                ? "Stored in Postgres and encrypted at rest when APP_SECRET is set."
                : "Memory mode: keys are lost when the proxy restarts."}
            </p>
          </div>

          <div className="glass rounded-2xl p-5">
            <div className="flex items-start justify-between gap-4">
              <div>
                <div className="flex items-center gap-2">
                  <RefreshCw className="size-4 text-accent" />
                  <h2 className="text-sm font-semibold">Round-robin</h2>
                </div>
                <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
                  {data?.roundRobin
                    ? "Requests rotate across every enabled key."
                    : "Every request uses the first enabled key."}
                </p>
              </div>
              <Toggle
                checked={data?.roundRobin ?? false}
                disabled={!data || roundRobin.isPending}
                label="Toggle round-robin"
                onChange={(next) => roundRobin.mutate(next)}
              />
            </div>
          </div>
        </div>

        <div className="glass overflow-hidden rounded-2xl">
          <div className="flex items-center justify-between border-b border-border/60 px-5 py-4">
            <div>
              <h2 className="text-sm font-semibold">Configured keys</h2>
              <p className="mt-0.5 text-[11px] text-muted-foreground">
                Secrets are masked; only the last four characters are shown.
              </p>
            </div>
            <Badge variant="outline" className="font-mono text-[10px] text-muted-foreground">
              {total} total
            </Badge>
          </div>

          {keys.isPending ? (
            <div className="flex items-center gap-2 px-5 py-10 text-sm text-muted-foreground">
              <Loader2 className="size-4 animate-spin" />
              Loading keys…
            </div>
          ) : total === 0 ? (
            <div className="px-5 py-12 text-center">
              <KeyRound className="mx-auto size-7 text-muted-foreground/60" />
              <p className="mt-3 text-sm font-medium">No API keys yet</p>
              <p className="mx-auto mt-1 max-w-xs text-xs text-muted-foreground">
                Add a Mistral key to start proxying requests.
              </p>
            </div>
          ) : (
            <ul className="divide-y divide-border/50">
              {data?.keys.map((key) => (
                <li key={key.id} className="flex flex-col gap-3 px-5 py-4 sm:flex-row sm:items-center">
                  <div className="flex min-w-0 flex-1 items-start gap-3">
                    <span
                      className={cn(
                        "mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-xl ring-1",
                        key.enabled
                          ? "bg-emerald-400/10 text-emerald-400 ring-emerald-400/20"
                          : "bg-secondary text-muted-foreground ring-border",
                      )}
                    >
                      <KeyRound className="size-4" />
                    </span>
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <p className="truncate text-sm font-medium">{key.label}</p>
                        <Badge
                          variant="outline"
                          className="h-4 px-1.5 font-mono text-[9px] tracking-wider text-muted-foreground uppercase"
                        >
                          {sourceLabel(key.source)}
                        </Badge>
                        {!key.enabled ? (
                          <Badge
                            variant="outline"
                            className="h-4 border-amber-400/25 bg-amber-400/10 px-1.5 text-[9px] text-amber-300"
                          >
                            DISABLED
                          </Badge>
                        ) : null}
                      </div>
                      <p className="mt-1 font-mono text-xs text-muted-foreground">{key.hint}</p>
                      <p className="mt-1 text-[11px] text-muted-foreground">
                        {key.stats.requests > 0 || key.stats.errors > 0 ? (
                          <>
                            {compactNumber(key.stats.requests)} requests ·{" "}
                            {compactNumber(key.stats.totalTokens)} tokens
                            {key.stats.lastUsed ? ` · last used ${timeAgo(key.stats.lastUsed)}` : ""}
                          </>
                        ) : (
                          "No traffic yet"
                        )}
                        {key.stats.errors > 0 ? (
                          <button
                            type="button"
                            className="ml-2 inline-flex items-center gap-1 text-red-300 transition-colors hover:text-red-200"
                            onClick={() => setOpenErrors(openErrors === key.id ? null : key.id)}
                          >
                            <AlertTriangle className="size-3" />
                            {key.stats.errors} error{key.stats.errors === 1 ? "" : "s"}
                            {openErrors === key.id ? " (hide)" : " (show)"}
                          </button>
                        ) : null}
                      </p>
                      {openErrors === key.id && key.errors.length > 0 ? (
                        <ul className="mt-2 space-y-1.5">
                          {key.errors.slice().reverse().map((err, index) => (
                            <li
                              key={index}
                              className="rounded-lg border border-destructive/20 bg-destructive/5 px-2.5 py-1.5 font-mono text-[10px] leading-relaxed text-red-300/90"
                            >
                              <span className="text-muted-foreground">{timeAgo(err.time)}</span>{" "}
                              {err.status ? <span className="text-red-300">HTTP {err.status}</span> : null}{" "}
                              {err.route}
                              {err.model ? ` · ${err.model}` : ""} — {err.message}
                            </li>
                          ))}
                        </ul>
                      ) : null}
                    </div>
                  </div>

                  <div className="flex items-center justify-between gap-3 sm:justify-end">
                    <div className="flex items-center gap-2">
                      <span className="text-[11px] text-muted-foreground">{key.enabled ? "Enabled" : "Disabled"}</span>
                      <Toggle
                        checked={key.enabled}
                        disabled={busyToggle}
                        label={`Toggle ${key.label}`}
                        onChange={(next) => update.mutate({ id: key.id, enabled: next })}
                      />
                    </div>
                    <div className="h-6 w-px bg-border/70" />
                    {pendingDelete === key.id ? (
                      <div className="flex items-center gap-1">
                        <Button
                          variant="destructive"
                          size="sm"
                          className="h-7 px-2 text-[11px]"
                          disabled={remove.isPending}
                          onClick={() => remove.mutate(key.id)}
                        >
                          {remove.isPending ? <Loader2 className="animate-spin" /> : "Delete"}
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 px-2 text-[11px]"
                          onClick={() => setPendingDelete(null)}
                        >
                          Cancel
                        </Button>
                      </div>
                    ) : (
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={`Delete ${key.label}`}
                        className="size-8 text-muted-foreground hover:text-destructive"
                        disabled={busyToggle}
                        onClick={() => setPendingDelete(key.id)}
                      >
                        <Trash2 className="size-4" />
                      </Button>
                    )}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </>
  )
}
