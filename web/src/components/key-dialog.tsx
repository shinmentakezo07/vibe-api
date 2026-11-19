import { useEffect, useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Eye, EyeOff, KeyRound, Loader2, ShieldCheck, Trash2 } from "lucide-react"
import { toast } from "sonner"

import { api, type Status } from "@/lib/api"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  status?: Status
}

export function KeyDialog({ open, onOpenChange, status }: Props) {
  const queryClient = useQueryClient()
  const [value, setValue] = useState("")
  const [visible, setVisible] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!open) {
      setValue("")
      setVisible(false)
      setError(null)
    }
  }, [open])

  const save = useMutation({
    mutationFn: api.saveKey,
    onSuccess: async (next) => {
      queryClient.setQueryData(["status"], next)
      await queryClient.invalidateQueries()
      toast.success("API key validated and applied")
      onOpenChange(false)
    },
    onError: (err: Error) => setError(err.message),
  })

  const clear = useMutation({
    mutationFn: api.clearKey,
    onSuccess: async (next) => {
      queryClient.setQueryData(["status"], next)
      await queryClient.invalidateQueries()
      toast.success("API key cleared")
      onOpenChange(false)
    },
    onError: (err: Error) => setError(err.message),
  })

  const busy = save.isPending || clear.isPending

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="glass border-white/10 sm:max-w-md">
        <DialogHeader>
          <div className="mb-2 flex size-11 items-center justify-center rounded-xl bg-primary/15 text-primary ring-1 ring-primary/30">
            <KeyRound className="size-5" />
          </div>
          <DialogTitle className="text-lg">Mistral API key</DialogTitle>
          <DialogDescription>
            {status?.storage === "postgres"
              ? "The key is checked against Mistral, encrypted when APP_SECRET is set, and stored in Postgres."
              : "The key is checked against Mistral, then used immediately by the proxy. No restart required."}
          </DialogDescription>
        </DialogHeader>

        {status?.keyConfigured ? (
          <div className="flex items-center justify-between rounded-xl border border-border/70 bg-secondary/40 px-3 py-2.5">
            <div className="flex items-center gap-2 text-sm">
              <ShieldCheck className="size-4 text-emerald-400" />
              <span className="text-muted-foreground">Active key</span>
              <span className="font-mono text-xs text-foreground">{status.keyHint}</span>
            </div>
            <Button
              variant="ghost"
              size="sm"
              className="text-muted-foreground hover:text-destructive"
              disabled={busy}
              onClick={() => clear.mutate()}
            >
              {clear.isPending ? <Loader2 className="animate-spin" /> : <Trash2 />}
              Clear
            </Button>
          </div>
        ) : null}

        <form
          className="space-y-3"
          onSubmit={(event) => {
            event.preventDefault()
            setError(null)
            save.mutate(value.trim())
          }}
        >
          <div className="space-y-2">
            <Label htmlFor="api-key">{status?.keyConfigured ? "Replace key" : "API key"}</Label>
            <div className="relative">
              <Input
                id="api-key"
                autoFocus
                autoComplete="off"
                spellCheck={false}
                type={visible ? "text" : "password"}
                placeholder="••••••••••••••••••••••••"
                className="h-11 pr-11 font-mono"
                value={value}
                onChange={(event) => setValue(event.target.value)}
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
            <p className="text-xs leading-relaxed text-muted-foreground">
              Create one in the{" "}
              <a
                href="https://console.mistral.ai/api-keys"
                target="_blank"
                rel="noreferrer"
                className="text-primary underline-offset-4 hover:underline"
              >
                Mistral console
              </a>
              .{" "}
              {status?.storage === "postgres"
                ? "It survives restarts; set APP_SECRET to encrypt it at rest."
                : "The key stays in proxy memory; restarting the proxy clears it."}
            </p>
          </div>

          {error ? (
            <p className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-red-300">
              {error}
            </p>
          ) : null}

          <DialogFooter className="pt-1">
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={busy || value.trim().length === 0}>
              {save.isPending ? <Loader2 className="animate-spin" /> : <ShieldCheck />}
              Validate &amp; save
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
