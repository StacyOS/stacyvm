"use client";

// Wait for a transaction receipt by hash. Useful when you already have a
// hash (e.g. from a server-submitted tx) and only need confirmations.

export { useWaitForTransactionReceipt as useWaitForTransaction } from "wagmi";
export type {
  UseWaitForTransactionReceiptParameters as UseWaitForTransactionParameters,
  UseWaitForTransactionReceiptReturnType as UseWaitForTransactionReturnType,
} from "wagmi";
