# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

@AGENTS.md

## Commands

```bash
pnpm dev         # start dev server (Turbopack, port 3000)
pnpm build       # production build (Turbopack)
pnpm start       # serve production build
pnpm lint        # run ESLint
```

No test suite is configured.

## File Structure

```
app/
  layout.tsx       # root layout — imports globals.css + RainbowKit styles, wraps in <Providers>
  page.tsx         # home page — renders <ConnectButton /> (Server Component)
  providers.tsx    # 'use client' — WagmiProvider > QueryClientProvider > RainbowKitProvider
  globals.css      # Tailwind v4 base styles
wagmi.ts           # wagmi + RainbowKit config (chains, projectId, SSR flag)
next.config.ts     # Next.js config (reactStrictMode only)
postcss.config.mjs # Tailwind v4 PostCSS plugin
```

New routes go under `app/`. New `'use client'` components that need wallet state can use wagmi hooks directly — the providers are already in scope from `layout.tsx`.

## Architecture

This is a Next.js 16 App Router frontend for EVM wallet connectivity. The entire app is a thin shell around RainbowKit.

**Provider tree** (`app/layout.tsx` → `app/providers.tsx` → children):
- `WagmiProvider` (config from `wagmi.ts`)
- `QueryClientProvider` (TanStack Query v5)
- `RainbowKitProvider`

`providers.tsx` is a `'use client'` component — it must stay that way because wagmi/RainbowKit rely on browser APIs.

**Chain configuration** (`wagmi.ts`): mainnet, polygon, optimism, arbitrum, base. Sepolia is conditionally added when `NEXT_PUBLIC_ENABLE_TESTNETS=true`.

The `projectId` in `wagmi.ts` is a placeholder (`'YOUR_PROJECT_ID'`). A real WalletConnect Cloud project ID is required for production.

## Next.js 16 Breaking Changes

This project runs Next.js **16.2.4** — significantly newer than training data. Key differences:

**Turbopack is the default** for both `next dev` and `next build`. No `--turbopack` flag needed. Use `--webpack` to opt out.

**Async Request APIs** — these are no longer synchronous and must be awaited:
- `cookies()`, `headers()`, `draftMode()`
- `params` and `searchParams` in page/layout/route files

```tsx
// ✓ correct
export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params
}
```

**`middleware` → `proxy`**: The middleware file must be renamed `proxy.ts`/`proxy.js` and the exported function renamed `proxy`. The `edge` runtime is not supported in proxy files (use `nodejs`). Config flags renamed too: `skipMiddlewareUrlNormalize` → `skipProxyUrlNormalize`.

**Caching APIs**:
- `unstable_cacheLife` / `unstable_cacheTag` → `cacheLife` / `cacheTag` (stable, no prefix)
- `revalidateTag` now requires a second argument: `revalidateTag('tag', 'max')`
- `updateTag` is a new Server Actions-only API for immediate cache expiry + refresh

**PPR**: `experimental.ppr` is removed. Use `cacheComponents: true` in `next.config.ts` instead.

**Slow navigations**: `<Suspense>` alone is not enough for instant client-side nav. Export `unstable_instant` from routes that should navigate instantly. Read `node_modules/next/dist/docs/01-app/02-guides/instant-navigation.md` before touching navigation.

**`turbopack` config**: moved from `experimental.turbopack` to top-level `turbopack` in `next.config.ts`.

## Tailwind CSS v4

This project uses Tailwind v4 (PostCSS plugin via `@tailwindcss/postcss`). The config format differs from v3 — there is no `tailwind.config.js`. Configuration is done in CSS using `@theme` directives. Read `node_modules/next/dist/docs/01-app/02-guides/tailwind-v3-css.md` if migrating from v3 patterns.
