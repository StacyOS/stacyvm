// Shared web3 types kept in one place so consumers don't import from `viem`
// and `wagmi` in scattered ways.

import type { Address, Hash, Hex } from "viem";

export type { Address, Hash, Hex };

// Minimal transaction lifecycle status used by the `useTransaction` hook.
export type TxStatus = "idle" | "pending" | "success" | "error";
