import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import "@fontsource-variable/geist"
import "./index.css"
import App from "./App.tsx"
import { Toaster } from "@/components/ui/sonner"

document.documentElement.classList.add("dark")

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 1_000,
      refetchOnWindowFocus: true,
      retry: 1,
    },
  },
})

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
      <Toaster theme="dark" position="bottom-right" richColors closeButton />
    </QueryClientProvider>
  </StrictMode>,
)
