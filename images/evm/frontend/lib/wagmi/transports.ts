// Viem transports for each supported chain.
//
// A transport is the network layer Wagmi/Viem use to talk to a chain.
// `http()` with no URL falls back to the chain's public RPC, which is fine
// for development. For production, set NEXT_PUBLIC_RPC_<CHAIN>_URL and the
// transport will use that endpoint instead.

import { http, type Transport } from "wagmi";
import { mainnet, sepolia } from "wagmi/chains";
import type { SupportedChainId } from "./chains";

// Read an optional RPC URL from the environment. Returning `undefined`
// makes `http()` fall back to the chain's default public RPC.
function rpcUrl(name: string): string | undefined {
  const v = process.env[`NEXT_PUBLIC_RPC_${name}_URL`];
  return v && v.length > 0 ? v : undefined;
}

export const transports: Record<SupportedChainId, Transport> = {
  [sepolia.id]: http(rpcUrl("SEPOLIA")),
  [mainnet.id]: http(rpcUrl("MAINNET")),
};
