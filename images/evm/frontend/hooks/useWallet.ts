"use client";

// `useWallet` — single, opinionated entry point for everything wallet-related.
// Wraps `useAccount` + `useConnect` + `useDisconnect` so components don't have
// to import three hooks and reconcile their state shapes.

import { useCallback } from "react";
import {
  useAccount,
  useConnect,
  useDisconnect,
  useConnectors,
  type Connector,
} from "wagmi";

export function useWallet() {
  const account = useAccount();
  const { connectAsync, isPending: isConnecting, error: connectError } = useConnect();
  const { disconnectAsync } = useDisconnect();
  const connectors = useConnectors();

  // Returns a promise so callers can await connection / catch errors
  // (user-rejected, no provider, etc.) close to where the click happens.
  const connect = useCallback(
    (connector: Connector) => connectAsync({ connector }),
    [connectAsync],
  );

  const disconnect = useCallback(() => disconnectAsync(), [disconnectAsync]);

  return {
    address: account.address,
    chain: account.chain,
    chainId: account.chainId,
    connector: account.connector,
    status: account.status,
    isConnected: account.isConnected,
    isConnecting: account.isConnecting || isConnecting,
    isReconnecting: account.isReconnecting,
    isDisconnected: account.isDisconnected,
    connectors,
    connect,
    disconnect,
    connectError,
  };
}
