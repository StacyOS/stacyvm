"use client";

// Higher-level transaction lifecycle helper.
//
// Why this exists: Wagmi splits a write flow into two hooks
// (`useWriteContract` to submit, `useWaitForTransactionReceipt` to confirm).
// Components usually want a unified status, so this hook composes both
// and exposes a single `status` plus a `reset()` to clear state between attempts.

import { useState, useCallback } from "react";
import { useWriteContract, useWaitForTransactionReceipt } from "wagmi";
import type { Hash } from "viem";
import type { TxStatus } from "@/lib/wagmi/types";

export function useTransaction() {
  const [hash, setHash] = useState<Hash | undefined>();

  const write = useWriteContract({
    mutation: {
      // Capture the hash as soon as the wallet returns it — that's our
      // signal to start polling for a receipt below.
      onSuccess: (txHash) => setHash(txHash),
    },
  });

  const receipt = useWaitForTransactionReceipt({
    hash,
    query: { enabled: Boolean(hash) },
  });

  // Collapse the two underlying mutations into a single status string.
  const status: TxStatus = receipt.isSuccess
    ? "success"
    : receipt.isError || write.isError
      ? "error"
      : write.isPending || receipt.isLoading
        ? "pending"
        : "idle";

  const reset = useCallback(() => {
    setHash(undefined);
    write.reset();
  }, [write]);

  return {
    // Submission
    writeContract: write.writeContract,
    writeContractAsync: write.writeContractAsync,
    // Lifecycle
    hash,
    receipt: receipt.data,
    status,
    isPending: status === "pending",
    isSuccess: status === "success",
    isError: status === "error",
    error: write.error ?? receipt.error,
    reset,
  };
}
