"use client";

// App-level providers entry point. Today this is just `Web3Provider`, but
// keeping a dedicated file makes it easy to add Theme/Toast/etc. later
// without churning `layout.tsx`.

import type { ReactNode } from "react";
import { Web3Provider } from "@/providers/Web3Provider";

export function Providers({ children }: { children: ReactNode }) {
  return <Web3Provider>{children}</Web3Provider>;
}
