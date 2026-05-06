"use client";

// `useSimulateContract` runs an `eth_call` against the contract with the
// caller's account so reverts surface BEFORE the user signs. The returned
// `request` is a fully-typed object you can pass to `writeContract` to
// guarantee the same parameters are used.
//
// Pattern:
//   const { data } = useSimulateContract({ address, abi, functionName, args });
//   if (data) await writeContractAsync(data.request);

export { useSimulateContract } from "wagmi";
export type {
  UseSimulateContractParameters,
  UseSimulateContractReturnType,
} from "wagmi";
