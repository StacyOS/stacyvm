// Centralized chain configuration.
//
// We use chain objects bundled with `wagmi/chains` because they include
// canonical chain IDs, native currency, and public RPC URLs out of the box.
// To add another chain, import it from `wagmi/chains` and append it to
// `supportedChains` — the rest of the app (transports, registry, hooks)
// is keyed off the chain IDs declared here, so it stays in sync automatically.

import { mainnet, sepolia } from "wagmi/chains";

// Sepolia is the default development network for this template.
// Mainnet is included so the same code path can target production without
// any structural changes.
export const supportedChains = [sepolia, mainnet] as const;

// Default chain used by hooks that need a chain hint when no wallet is connected.
export const defaultChain = sepolia;

// Narrow union of supported chain IDs — used everywhere we key things by chain.
export type SupportedChainId = (typeof supportedChains)[number]["id"];
