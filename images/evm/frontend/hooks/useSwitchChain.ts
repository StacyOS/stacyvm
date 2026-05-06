"use client";

// Re-export — Wagmi already exposes the exact shape we want.
// Use it like: `const { switchChain } = useSwitchChain(); switchChain({ chainId });`

export { useSwitchChain } from "wagmi";
export type {
  UseSwitchChainParameters,
  UseSwitchChainReturnType,
} from "wagmi";
