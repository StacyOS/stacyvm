// Wagmi config — the single source of truth for chains, transports, and connectors.
//
// We deliberately use only injected-style connectors (browser wallets like
// MetaMask, Brave Wallet, Rabby, etc.) so the template requires zero external
// project setup: no WalletConnect project ID, no Web3Modal, no RainbowKit.
// To add a new wallet kind, import a connector from `wagmi/connectors` and
// append it to `connectors` below.

import { createConfig } from "wagmi";
import { injected, metaMask } from "wagmi/connectors";
import { supportedChains } from "./chains";
import { transports } from "./transports";

// `ssr: true` lets the Wagmi provider mount safely in Next.js App Router
// without referencing `window` during server rendering. Reconnection happens
// on the client after hydration via `useReconnect` (wired in Web3Provider).
export const wagmiConfig = createConfig({
  chains: supportedChains,
  connectors: [
    // Generic injected catches anything that injects EIP-1193 (Brave, Rabby, etc.).
    injected({ shimDisconnect: true }),
    // Explicit MetaMask connector gives a stable id/icon and avoids the
    // ambiguity multiple injected providers can introduce.
    metaMask(),
  ],
  transports,
  ssr: true,
});

// Augments Wagmi's internal `Register` interface so hook return types are
// inferred against this exact config (chains + connectors). This is what
// makes `useAccount().chain` typed as one of our supported chains.
declare module "wagmi" {
  interface Register {
    config: typeof wagmiConfig;
  }
}
