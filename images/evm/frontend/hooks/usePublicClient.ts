"use client";

// Public client = read-only Viem client bound to a chain.
// Use it for ad-hoc reads or actions Wagmi doesn't wrap (e.g. `getLogs`,
// `multicall` with custom batching, `getProof`).

export { usePublicClient } from "wagmi";
export type { UsePublicClientParameters, UsePublicClientReturnType } from "wagmi";
