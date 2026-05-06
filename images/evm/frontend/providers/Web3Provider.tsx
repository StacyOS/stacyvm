"use client";

// Composes Wagmi + TanStack Query so the rest of the app can use any
// web3 hook without per-page setup. Mounted once near the root in
// `app/layout.tsx`.
//
// Why a separate Client Component? The Wagmi provider relies on browser
// APIs and React context, both of which require `'use client'`. Keeping
// only this file client-side preserves Server Components everywhere else.

import { useState, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { WagmiProvider } from "wagmi";
import { wagmiConfig } from "@/lib/wagmi/config";

interface Web3ProviderProps {
  children: ReactNode;
}

export function Web3Provider({ children }: Web3ProviderProps) {
  // useState ensures one QueryClient per browser session and avoids
  // re-creating it on every render (which would lose all cached queries).
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            // RPC reads are typically expensive; stale time avoids
            // re-fetching on every component mount.
            // We don't want hooks to silently retry forever on a bad
            // RPC URL or a reverted contract call.
          },
        },
      }),
  );

  return (
    <WagmiProvider config={wagmiConfig}>
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    </WagmiProvider>
  );
}
