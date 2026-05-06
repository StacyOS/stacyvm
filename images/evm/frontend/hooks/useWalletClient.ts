"use client";

// Wallet client = signer-bound Viem client. Use only inside event handlers
// or effects that run after the user connects, since this is undefined
// until a wallet is ready.
//
// Wagmi exposes this via `useConnectorClient`; we alias it for clarity.

export { useConnectorClient as useWalletClient } from "wagmi";
export type {
  UseConnectorClientParameters as UseWalletClientParameters,
  UseConnectorClientReturnType as UseWalletClientReturnType,
} from "wagmi";
