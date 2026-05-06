"use client";

// Compact status badge — useful inside cards/dashboards where you want a
// quick "are we connected, on the right chain, with what balance" readout.

import { useWallet } from "@/hooks/useWallet";
import { useBalance } from "@/hooks/useBalance";
import { useNetwork } from "@/hooks/useNetwork";
import { shortenAddress, formatBalance } from "@/lib/format";

export function WalletStatus() {
  const { address, isConnected, isReconnecting } = useWallet();
  const { chain, isSupported } = useNetwork();
  const balance = useBalance();

  if (!isConnected || !address) {
    return (
      <div className="flex items-center gap-2 text-sm text-zinc-500 dark:text-zinc-400">
        <span className="h-1.5 w-1.5 rounded-full bg-zinc-400" />
        {isReconnecting ? "Reconnecting…" : "Wallet not connected"}
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-1 text-sm">
      <div className="flex items-center gap-2">
        <span
          className={`h-1.5 w-1.5 rounded-full ${isSupported ? "bg-emerald-500" : "bg-red-500"}`}
        />
        <span className="font-medium text-zinc-900 dark:text-zinc-50">
          {shortenAddress(address)}
        </span>
        <span className="text-xs text-zinc-500 dark:text-zinc-400">
          on {chain?.name ?? "unknown"}
        </span>
      </div>
      <div className="text-xs text-zinc-500 dark:text-zinc-400">
        {balance.data
          ? `${formatBalance(balance.data.value, balance.data.decimals)} ${balance.data.symbol}`
          : "—"}
      </div>
    </div>
  );
}
