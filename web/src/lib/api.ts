export type Status = {
  keyConfigured: boolean
  keyHint: string
  anthropicModel: string
  proxyAuth: boolean
  upstream: string
}

export type Model = {
  id: string
  name?: string
  description?: string
  max_context_length?: number
  capabilities?: Record<string, boolean>
  aliases?: string[]
  type?: string
  owned_by?: string
}

export type ModelList = {
  data: Model[]
}

export type UsageTotals = {
  requests: number
  promptTokens: number
  completionTokens: number
  totalTokens: number
}

export type UsageRecord = {
  id: string
  time: string
  route: string
  model: string
  promptTokens: number
  completionTokens: number
  totalTokens: number
  stream: boolean
  cachedInputTokens: number
}

export type UsageSnapshot = {
  since: string
  totals: UsageTotals
  byModel: ({ model: string } & UsageTotals)[]
  recent: UsageRecord[]
  lastEvent?: UsageRecord
}

type ErrorEnvelope = {
  error?: { message?: string; type?: string }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...init?.headers,
    },
  })
  const text = await response.text()
  let payload: unknown = null
  try {
    payload = text ? JSON.parse(text) : null
  } catch {
    payload = null
  }
  if (!response.ok) {
    const message = (payload as ErrorEnvelope | null)?.error?.message
    throw new Error(message || `Request failed (${response.status})`)
  }
  return payload as T
}

export const api = {
  status: () => request<Status>("/api/status"),
  usage: () => request<UsageSnapshot>("/api/usage"),
  models: () => request<ModelList>("/v1/models"),
  saveKey: (apiKey: string) =>
    request<Status>("/api/key", {
      method: "POST",
      body: JSON.stringify({ apiKey }),
    }),
  clearKey: () => request<Status>("/api/key", { method: "DELETE" }),
}

export function compactNumber(value: number): string {
  return new Intl.NumberFormat("en", {
    notation: "compact",
    maximumFractionDigits: 1,
  }).format(value)
}

export function formatNumber(value: number): string {
  return new Intl.NumberFormat("en").format(value)
}

export function timeAgo(iso: string): string {
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(iso).getTime()) / 1000))
  if (seconds < 5) return "now"
  if (seconds < 60) return `${seconds}s ago`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  return `${Math.floor(hours / 24)}d ago`
}
