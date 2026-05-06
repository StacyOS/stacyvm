"use client";

// Wallet selection modal.
//
// Strategy: list every connector Wagmi exposes, but only enable the ones
// whose injected provider is actually present (`connector.getProvider()`
// resolves). If no injected wallets are detected at all, show a friendly
// install prompt instead of an empty list — this is the no-WalletConnect
// fallback strategy referenced in `docs/web3-template.md`.

import { useEffect, useMemo, useState } from "react";
import type { Connector } from "wagmi";
import { useWallet } from "@/hooks/useWallet";
import { Button } from "@/components/ui/Button";
import { Dialog } from "@/components/ui/Dialog";

interface WalletModalProps {
  open: boolean;
  onClose: () => void;
}

export function WalletModal({ open, onClose }: WalletModalProps) {
  const { connectors, connect, isConnecting, connectError } = useWallet();

  // Track which connectors have a live provider in this browser. We probe
  // once when the modal opens; results drive the disabled state below.
  const [available, setAvailable] = useState<Set<string>>(new Set());
  const [pendingId, setPendingId] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    (async () => {
      const results = await Promise.all(
        connectors.map(async (c) => {
          try {
            const provider = await c.getProvider();
            return provider ? c.uid : null;
          } catch {
            return null;
          }
        }),
      );
      if (!cancelled) setAvailable(new Set(results.filter(Boolean) as string[]));
    })();
    return () => {
      cancelled = true;
    };
  }, [open, connectors]);

  // Deduplicate by `name` so multiple injected providers don't show a
  // wall of identical "Injected" entries.
  const visibleConnectors = useMemo(() => {
    const seen = new Set<string>();
    return connectors.filter((c) => {
      if (seen.has(c.name)) return false;
      seen.add(c.name);
      return true;
    });
  }, [connectors]);

  const hasAnyInjected = available.size > 0;

  async function handleConnect(connector: Connector) {
    setPendingId(connector.uid);
    try {
      await connect(connector);
      onClose();
    } catch {
      // User-rejected and "no provider" errors surface via `connectError`
      // below; we just need to stop showing the spinner.
    } finally {
      setPendingId(null);
    }
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="Connect a wallet"
      description="Choose an installed browser wallet to continue."
    >
      {!hasAnyInjected ? (
        <div className="rounded-xl border border-dashed border-zinc-300 bg-zinc-50 p-4 text-sm text-zinc-700 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300">
          No browser wallet detected. Install one to continue:
          <div className="mt-3 flex flex-wrap gap-2">
            <a
              href="https://metamask.io/download/"
              target="_blank"
              rel="noreferrer"
              className="rounded-full bg-zinc-900 px-3 py-1.5 text-xs font-medium text-white hover:bg-zinc-800 dark:bg-white dark:text-zinc-900 dark:hover:bg-zinc-200"
            >
              Get MetaMask
            </a>
            <a
              href="https://brave.com/wallet/"
              target="_blank"
              rel="noreferrer"
              className="rounded-full border border-zinc-300 px-3 py-1.5 text-xs font-medium text-zinc-800 hover:bg-zinc-100 dark:border-zinc-700 dark:text-zinc-100 dark:hover:bg-zinc-800"
            >
              Brave Wallet
            </a>
          </div>
        </div>
      ) : (
        <ul className="flex flex-col gap-2">
          {visibleConnectors.map((connector) => {
            const isAvailable = available.has(connector.uid);
            const isPending = pendingId === connector.uid;
            return (
              <li key={connector.uid}>
                <button
                  onClick={() => handleConnect(connector)}
                  disabled={!isAvailable || isConnecting}
                  className="group flex w-full items-center gap-3 rounded-xl border border-zinc-200 bg-white p-3 text-left transition-all hover:border-zinc-300 hover:bg-zinc-50 disabled:cursor-not-allowed disabled:opacity-60 dark:border-zinc-800 dark:bg-zinc-950 dark:hover:border-zinc-700 dark:hover:bg-zinc-900"
                >
                  <span
                    aria-hidden
                    className="flex h-9 w-9 items-center justify-center rounded-lg bg-gradient-to-br from-indigo-500 to-purple-500 text-sm font-bold text-white"
                  >
                    {connector.name.charAt(0)}
                  </span>
                  <span className="flex-1">
                    <span className="block text-sm font-medium text-zinc-900 dark:text-zinc-50">
                      {connector.name}
                    </span>
                    <span className="block text-xs text-zinc-500 dark:text-zinc-400">
                      {isAvailable ? "Detected" : "Not installed"}
                    </span>
                  </span>
                  {isPending && (
                    <span
                      aria-hidden
                      className="h-4 w-4 rounded-full border-2 border-zinc-400 border-t-transparent animate-spin"
                    />
                  )}
                </button>
              </li>
            );
          })}
        </ul>
      )}

      {connectError && (
        <p className="mt-4 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-950/40 dark:text-red-400">
          {connectError.message}
        </p>
      )}

      <div className="mt-5 flex justify-end">
        <Button variant="ghost" size="sm" onClick={onClose}>
          Cancel
        </Button>
      </div>
    </Dialog>
  );
}
