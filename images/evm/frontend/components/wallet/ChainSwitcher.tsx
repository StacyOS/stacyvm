"use client";

import { Button } from "@/components/ui/Button";
import { useWallet, useNetwork, useSwitchChain } from "@/hooks";
import { supportedChains } from "@/lib/wagmi/chains";

export function ChainSwitcher() {
  const { isConnected } = useWallet();
  const { chain, isSupported } = useNetwork();
  const { switchChain } = useSwitchChain();

  return (
    <>
      <div className="flex flex-wrap gap-2">
        {supportedChains.map((c) => (
          <Button
            key={c.id}
            size="sm"
            variant={chain?.id === c.id ? "primary" : "secondary"}
            onClick={() => switchChain({ chainId: c.id })}
            disabled={!isConnected}
          >
            {c.name}
          </Button>
        ))}
      </div>
      {!isSupported && isConnected && (
        <p className="mt-3 text-xs text-red-600 dark:text-red-400">
          You&apos;re on an unsupported network. Switch above.
        </p>
      )}
    </>
  );
}
