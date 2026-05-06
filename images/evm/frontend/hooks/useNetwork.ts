"use client";

// `useNetwork` — chain metadata for the currently-connected wallet, plus
// the full list of chains the app supports. Wagmi v2/v3 removed the
// original `useNetwork` hook in favor of `useChainId` + `useChains`; this
// recreates the convenient combined shape.

import { useAccount, useChainId, useChains } from "wagmi";

export function useNetwork() {
  const chainId = useChainId();
  const chains = useChains();
  const { chain } = useAccount();

  // `chain` from `useAccount` may be undefined when disconnected — fall
  // back to looking up the active chain ID against the supported list.
  const activeChain = chain ?? chains.find((c) => c.id === chainId);
  const isSupported = chains.some((c) => c.id === chainId);

  return {
    chain: activeChain,
    chainId,
    chains,
    isSupported,
  };
}
