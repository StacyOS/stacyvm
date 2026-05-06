# EVM dApp Template — Developer Guide

A minimal Next.js + Wagmi + Viem + TanStack Query starter, configured for
**injected wallets only** with **Sepolia as the default chain**. Pass your
contract's ABI and address directly into the hooks at the call site.

> This repo is intended to be wired up by agents that already have a
> deployed contract's ABI and address. It deliberately ships **no**
> predefined contract templates, ABI files, or address registries — those
> would only invite hallucinated assumptions about a contract that hasn't
> actually been deployed.

---

## What this gives you

- **No external wallet kit.** No WalletConnect project ID, no Web3Modal,
  no RainbowKit. Connection is via browser-injected wallets (MetaMask,
  Brave, Rabby, anything EIP-1193) — zero account/setup friction.
- **One source of truth** for chains and transports.
- **Reusable hooks** that mirror your mental model: `useWallet`,
  `useBalance`, `useContractRead`, `useContractWrite`, `useTransaction`,
  `useSwitchChain`, etc.
- **Strong TypeScript** end-to-end: `as const` ABIs preserve literal
  types, typed `chainId`, inferred `functionName` and `args`, typed
  return values.

---

## Why Wagmi + Viem (and why not WalletConnect)

- **Wagmi** owns React state for accounts/connectors and ships hooks that
  wrap multicall reads, transaction submission, and wallet events.
- **Viem** is the typed EVM client underneath — `Address`, `Hash`, ABI
  parsing, transports, encoding. Anything Wagmi doesn't wrap, you can do
  via `usePublicClient()` or `useWalletClient()`.
- **No WalletConnect** because:
  - It requires a registered project ID (extra signup friction).
  - QR/relay flows are heavier than most teams need to start.
  - Injected wallets cover ~all desktop dApp users out of the box.

If you later need WalletConnect: install `@wagmi/connectors`'s
`walletConnect`, add it to `lib/wagmi/config.ts`, and supply the project ID
via `NEXT_PUBLIC_WALLETCONNECT_PROJECT_ID`. No other code changes needed.

---

## Project layout

```text
app/                      Next.js App Router pages + layout
  layout.tsx              Wraps the tree in <Providers> (client component)
  page.tsx                Demo landing page (wallet + network only)
  providers.tsx           Composition point for app-level providers

components/
  ui/                     Button, Card, Dialog primitives (Tailwind only)
  wallet/                 ConnectButton, WalletModal, WalletStatus

hooks/                    One web3 hook per file, barrel-exported via index.ts

lib/
  wagmi/
    chains.ts             supportedChains + defaultChain
    transports.ts         http() per chain, env override
    config.ts             createConfig(): chains + connectors + transports
    types.ts              Shared types (Address, Hash, TxStatus)
  format.ts               shortenAddress, formatBalance

providers/
  Web3Provider.tsx        WagmiProvider + QueryClientProvider

docs/web3-template.md     This file
```

---

## How to use

### Connect a wallet

The `<ConnectButton />` (in `components/wallet/`) handles everything:
opens a modal, lists detected injected providers, falls back to install
links, and displays the connected address + chain when done.

```tsx
import { ConnectButton } from "@/components/wallet/ConnectButton";

export default function Header() {
  return <ConnectButton />;
}
```

### Read the connected account

```tsx
import { useWallet } from "@/hooks";

const { address, isConnected, chain, disconnect } = useWallet();
```

### Read a native balance

```tsx
import { useBalance } from "@/hooks";

const { data } = useBalance(); // current account, current chain
// data.value, data.decimals, data.symbol
```

### Read a contract

Define the ABI with `as const` (so Wagmi can infer typed function names,
args, and return values) and pass it inline with the address.

```tsx
import { useContractRead } from "@/hooks";

const myAbi = [
  {
    type: "function",
    name: "balanceOf",
    stateMutability: "view",
    inputs: [{ name: "account", type: "address" }],
    outputs: [{ type: "uint256" }],
  },
] as const;

const address = "0x..." as const;

const { data } = useContractRead({
  address,
  abi: myAbi,
  functionName: "balanceOf",
  args: [account],
});
```

### Write a contract (with lifecycle)

```tsx
import { useTransaction } from "@/hooks";

const tx = useTransaction();

async function onClick() {
  await tx.writeContractAsync({
    address,
    abi: myAbi,
    functionName: "transfer",
    args: [to, amount],
  });
}
// tx.status: "idle" | "pending" | "success" | "error"
// tx.hash, tx.receipt, tx.error, tx.reset()
```

### Simulate before sending

```tsx
import { useSimulateContract } from "@/hooks";

const { data } = useSimulateContract({
  address, abi: myAbi, functionName: "transfer", args: [to, amount],
});
// pass data.request straight to writeContractAsync to avoid revert surprises
```

### Switch chains

```tsx
import { useSwitchChain } from "@/hooks";
import { sepolia } from "wagmi/chains";

const { switchChain } = useSwitchChain();
switchChain({ chainId: sepolia.id });
```

---

## Adding to the template

### Add a new chain

1. Import the chain from `wagmi/chains` in `lib/wagmi/chains.ts` and append
   it to `supportedChains`.
2. Add a transport for it in `lib/wagmi/transports.ts`.
3. (Optional) Add per-chain RPC override via `NEXT_PUBLIC_RPC_<NAME>_URL`.

### Add a new hook

Create `hooks/useXyz.ts`, mark it `"use client"`, and re-export from
`hooks/index.ts`. Prefer composing existing Wagmi hooks; only build a
wrapper when it materially reduces caller boilerplate.

### Replace transports / add a custom RPC

Edit `lib/wagmi/transports.ts`. Use `fallback([http(a), http(b)])` for
multi-RPC redundancy or `webSocket(url)` for streaming subscriptions.

---

## Fallback strategies

| Situation | What the template does |
| --- | --- |
| No injected wallet | `WalletModal` shows install links (MetaMask, Brave). |
| User rejects connect | `useConnect` returns the error; surfaced in modal. |
| Wrong / unsupported chain | `useNetwork().isSupported` → red pill + switch UI. |
| RPC failure | TanStack Query retries once; error surfaces via hook return. |
| Contract revert on write | Caught by `useSimulateContract` if used; otherwise surfaces via `tx.error`. |

---

## "If not like this, then how?" — design choices

### No predefined contract registry

- **Default:** pass ABI + address inline at the call site. Nothing
  hard-coded in this repo. Avoids agents hallucinating a `useContract("foo")`
  that maps to a contract that was never deployed.
- **Alternative:** if your app has many contracts and you want one source
  of truth, build a `lib/contracts/` registry tailored to *your*
  deployments — it's not in this starter because every project's needs
  differ.

### Injected-only connectors

- **Default:** `injected()` + `metaMask()`. Zero external setup.
- **Alternative:** add WalletConnect / Coinbase Wallet later (one line in
  `lib/wagmi/config.ts`). Preserves the rest of the architecture.

### Composed `useTransaction` hook vs raw Wagmi hooks

- **Default:** `useTransaction()` collapses submit + wait into one
  status string for simpler UIs.
- **Alternative:** use `useWriteContract` + `useWaitForTransactionReceipt`
  directly when you need finer-grained control (e.g. optimistic updates).

### Cookie-based SSR hydration

- **Default:** `ssr: true` in the config; reconnection happens on the
  client. Simple, no `cookies()` plumbing.
- **Alternative:** `cookieStorage` + `cookieToInitialState` for instant
  authenticated render. Worth adding when SEO/initial paint matters more
  than current vs. ~50ms reconnect on page load.

---

## Environment variables

All optional.

```bash
NEXT_PUBLIC_RPC_SEPOLIA_URL=        # custom RPC for Sepolia
NEXT_PUBLIC_RPC_MAINNET_URL=        # custom RPC for Mainnet
```
