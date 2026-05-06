@AGENTS.md

# EVM dApp Starter

This project is a Next.js + Wagmi + Viem + TanStack Query starter for EVM
dApps. **Sepolia is the default chain.** Connection uses **injected wallets
only** (no WalletConnect, no external wallet kits, no project IDs).

This repository is intended to be wired up by an agent that already has the
deployed contract's ABI and address. There are intentionally **no
predefined contract templates, ABIs, address registries, or example
contracts** in this repo — pass ABI + address straight into the hooks.

## Architecture

- **`lib/wagmi/`** — Single source of truth for chains, transports, and the
  Wagmi config. To add a chain: edit `chains.ts` + `transports.ts`.
- **`lib/format.ts`** — Pure display helpers (`shortenAddress`,
  `formatBalance`).
- **`hooks/`** — One hook per file, all `"use client"`, barrel-exported via
  `hooks/index.ts`. Prefer composing Wagmi hooks; only wrap when it
  materially reduces caller boilerplate.
- **`components/wallet/`** — `ConnectButton`, `WalletModal`, `WalletStatus`.
  Built on injected connectors with an install-fallback UI.
- **`providers/Web3Provider.tsx`** — `WagmiProvider` + `QueryClientProvider`.
  Mounted via `app/providers.tsx` from `app/layout.tsx`.

## Conventions

- ABIs supplied by callers must be declared with `as const` so Wagmi/Viem
  preserve literal types for `functionName`, `args`, and return values.
- New chains: update both `supportedChains` (in `lib/wagmi/chains.ts`) and
  `transports` (in `lib/wagmi/transports.ts`). The `SupportedChainId`
  union is derived from the former.
- Server Components by default. Anything that touches Wagmi/QueryClient
  hooks must be `"use client"`.
- Optional env override:
  - `NEXT_PUBLIC_RPC_<CHAIN>_URL` — custom RPC per chain.

## Integrating a contract

Do not add registries, name → address maps, or pre-baked ABI files. Pass
the ABI and address directly to the contract hooks at the call site:

```tsx
import { useContractRead, useTransaction } from "@/hooks";

const myAbi = [/* ... */] as const;
const address = "0x..." as const;

const { data } = useContractRead({ address, abi: myAbi, functionName: "..." });
const tx = useTransaction();
await tx.writeContractAsync({ address, abi: myAbi, functionName: "..." });
```

See `docs/web3-template.md` for the full developer guide.
