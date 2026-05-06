"use client";

// Polished wallet button — the user-facing entry to the entire wallet flow.
//
// Disconnected → opens `WalletModal` to pick a connector.
// Connected    → shows network + truncated address; click opens an
//                "account" panel inside the same modal pattern.

import { useState } from "react";
import { useWallet } from "@/hooks/useWallet";
import { useNetwork } from "@/hooks/useNetwork";
import { useBalance } from "@/hooks/useBalance";
import { Button } from "@/components/ui/Button";
import { Dialog } from "@/components/ui/Dialog";
import { WalletModal } from "./WalletModal";
import { shortenAddress, formatBalance } from "@/lib/format";

export function ConnectButton() {
  const { address, isConnected, isConnecting, isReconnecting, disconnect } = useWallet();
  const { chain, isSupported } = useNetwork();
  const balance = useBalance();

  const [connectOpen, setConnectOpen] = useState(false);
  const [accountOpen, setAccountOpen] = useState(false);

  // Disconnected state — primary call-to-action.
  if (!isConnected || !address) {
    return (
      <>
        <Button
          onClick={() => setConnectOpen(true)}
          loading={isConnecting || isReconnecting}
        >
          {isReconnecting ? "Reconnecting…" : "Connect Wallet"}
        </Button>
        <WalletModal open={connectOpen} onClose={() => setConnectOpen(false)} />
      </>
    );
  }

  // Connected state.
  return (
    <>
      <div className="inline-flex items-center gap-1 rounded-full border border-zinc-200 bg-white p-1 shadow-sm dark:border-zinc-800 dark:bg-zinc-950">
        {/* Network pill — surfaces unsupported-chain state in red. */}
        <span
          className={`hidden sm:inline-flex items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-medium ${
            isSupported
              ? "bg-zinc-100 text-zinc-700 dark:bg-zinc-900 dark:text-zinc-300"
              : "bg-red-50 text-red-700 dark:bg-red-950/40 dark:text-red-400"
          }`}
        >
          <span
            className={`h-1.5 w-1.5 rounded-full ${
              isSupported ? "bg-emerald-500" : "bg-red-500"
            }`}
            aria-hidden
          />
          {chain?.name ?? "Unsupported"}
        </span>
        {/* Address pill — clicking opens the account dialog. */}
        <button
          onClick={() => setAccountOpen(true)}
          className="rounded-full bg-zinc-900 px-4 py-1.5 text-xs font-medium text-white hover:bg-zinc-800 dark:bg-white dark:text-zinc-900 dark:hover:bg-zinc-200"
        >
          {shortenAddress(address)}
        </button>
      </div>

      <Dialog
        open={accountOpen}
        onClose={() => setAccountOpen(false)}
        title="Account"
      >
        <div className="space-y-4">
          <div className="rounded-xl bg-zinc-50 p-4 dark:bg-zinc-900">
            <div className="text-xs text-zinc-500 dark:text-zinc-400">Address</div>
            <div className="mt-0.5 font-mono text-sm text-zinc-900 dark:text-zinc-50">
              {address}
            </div>
          </div>
          <div className="rounded-xl bg-zinc-50 p-4 dark:bg-zinc-900">
            <div className="text-xs text-zinc-500 dark:text-zinc-400">Balance</div>
            <div className="mt-0.5 text-sm text-zinc-900 dark:text-zinc-50">
              {balance.data
                ? `${formatBalance(balance.data.value, balance.data.decimals)} ${balance.data.symbol}`
                : "—"}
            </div>
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setAccountOpen(false)}>
              Close
            </Button>
            <Button
              variant="danger"
              onClick={async () => {
                await disconnect();
                setAccountOpen(false);
              }}
            >
              Disconnect
            </Button>
          </div>
        </div>
      </Dialog>
    </>
  );
}
