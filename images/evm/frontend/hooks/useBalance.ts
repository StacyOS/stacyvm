"use client";

// Native-token balance hook. Defaults to the connected account but can be
// pointed at any address via `address`. Pass `chainId` to read on a specific
// chain regardless of which one the wallet is currently on.

import { useAccount, useBalance as useWagmiBalance } from "wagmi";
import type { Address } from "viem";
import type { SupportedChainId } from "@/lib/wagmi/chains";

interface UseBalanceParams {
  address?: Address;
  chainId?: SupportedChainId;
}

export function useBalance({ address, chainId }: UseBalanceParams = {}) {
  const { address: connected } = useAccount();
  const target = address ?? connected;

  // `query.enabled` short-circuits the request when there's no address yet,
  // so this hook is safe to call before the wallet connects.
  return useWagmiBalance({
    address: target,
    chainId,
    query: { enabled: Boolean(target) },
  });
}
