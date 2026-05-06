// Display helpers for addresses and balances. Pure and dependency-light
// so they can be used from server components, client components, or hooks.

import { formatUnits } from "viem";
import type { Address } from "viem";

// Shorten an address for compact UI display: 0x1234…abcd
export function shortenAddress(address: Address | string, chars = 4): string {
  if (!address) return "";
  return `${address.slice(0, 2 + chars)}…${address.slice(-chars)}`;
}

// Format a raw token balance (as bigint) using its decimals, then trim
// trailing zeros so "1.000000" becomes "1".
export function formatBalance(
  value: bigint | undefined,
  decimals = 18,
  maxFractionDigits = 4,
): string {
  if (value === undefined) return "0";
  const formatted = formatUnits(value, decimals);
  const [whole, frac = ""] = formatted.split(".");
  const trimmed = frac.slice(0, maxFractionDigits).replace(/0+$/, "");
  return trimmed.length > 0 ? `${whole}.${trimmed}` : whole;
}
