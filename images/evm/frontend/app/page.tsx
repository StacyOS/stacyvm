"use client";

// Landing page — wallet connect + network switching, no contract logic.
// Wire your contracts in by passing an ABI and address directly to the
// `useContractRead` / `useContractWrite` / `useTransaction` hooks.

import dynamic from "next/dynamic";
import { ConnectButton } from "@/components/wallet/ConnectButton";
import { WalletStatus } from "@/components/wallet/WalletStatus";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/ui/Card";
import { defaultChain } from "@/lib/wagmi/chains";

// ChainSwitcher reads wallet + chain state that differs between server and
// client renders — skip SSR entirely to avoid hydration mismatches.
const ChainSwitcher = dynamic(
  () => import("@/components/wallet/ChainSwitcher").then((m) => m.ChainSwitcher),
  { ssr: false },
);

export default function Home() {
  return (
    <div className="flex flex-1 flex-col">
      <header className="border-b border-zinc-200 dark:border-zinc-800">
        <div className="mx-auto flex w-full max-w-5xl items-center justify-between px-6 py-4">
          <div>
            <div className="text-sm font-semibold tracking-tight">EVM dApp Starter</div>
            <div className="text-xs text-zinc-500 dark:text-zinc-400">
              Next.js · Wagmi · Viem · TanStack Query
            </div>
          </div>
          <ConnectButton />
        </div>
      </header>

      <main className="mx-auto w-full max-w-5xl flex-1 px-6 py-10">
        <div className="mb-10">
          <h1 className="text-3xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-50">
            Build EVM apps without the boilerplate.
          </h1>
          <p className="mt-2 max-w-2xl text-sm text-zinc-600 dark:text-zinc-400">
            Wallet connection and chain switching are wired up. Pass your own
            ABI and address into the contract hooks to integrate. Default
            network: {defaultChain.name}.
          </p>
        </div>

        <div className="grid gap-4 md:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>Wallet</CardTitle>
              <CardDescription>Live status of the connected account.</CardDescription>
            </CardHeader>
            <WalletStatus />
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Network</CardTitle>
              <CardDescription>
                Switch between supported chains in one click.
              </CardDescription>
            </CardHeader>
            <ChainSwitcher />
          </Card>
        </div>

        <div className="mt-10 text-xs text-zinc-500 dark:text-zinc-400">
          See <code className="font-mono">docs/web3-template.md</code> for the full guide.
        </div>
      </main>
    </div>
  );
}
