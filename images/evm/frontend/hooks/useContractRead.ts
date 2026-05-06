"use client";

// Thin alias around Wagmi's `useReadContract` that re-exports it under a
// name that matches our hook taxonomy. We intentionally do NOT add a
// wrapping layer here — Wagmi already returns a fully-typed result, and
// any extra abstraction would erode the literal-type inference that powers
// `functionName` / `args` autocomplete.
//
// Usage:
//   const { data } = useContractRead({
//     address, abi, functionName: "balanceOf", args: [user],
//   });

export { useReadContract as useContractRead } from "wagmi";
export type {
  UseReadContractParameters as UseContractReadParameters,
  UseReadContractReturnType as UseContractReadReturnType,
} from "wagmi";
