"use client";

// Re-export of `useWriteContract` under the project-wide naming convention.
// As with `useContractRead`, we keep this a 1:1 alias to preserve the
// strong type inference Wagmi/Viem provide on `functionName` and `args`.
//
// Typical pattern:
//   const { writeContractAsync, isPending } = useContractWrite();
//   await writeContractAsync({ address, abi, functionName: "increment" });

export { useWriteContract as useContractWrite } from "wagmi";
export type {
  UseWriteContractParameters as UseContractWriteParameters,
  UseWriteContractReturnType as UseContractWriteReturnType,
} from "wagmi";
