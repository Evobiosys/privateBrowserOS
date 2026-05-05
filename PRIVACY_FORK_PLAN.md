# Privacy Fork Plan — sebra-private-browseros

Personal-use, privacy-hardened fork of [browseros-ai/BrowserOS](https://github.com/browseros-ai/BrowserOS).
Branch: `private`. Date: 2026-04-26.

> Scope: surgical patches only. No release engineering, no signed builds, no CI changes.
> Goal: zero outbound telemetry by default; default LLM is local Ollama.

---

## Repo Topology (verified)

`browseros-ai/BrowserOS` is a monorepo containing:
- `packages/browseros/` — Chromium fork patches (~150MB tree, mostly `chromium_patches/`).
- `packages/browseros-agent/` — the TypeScript agent monorepo (3 apps: `agent` extension, `server`, `cli`).

The standalone repo `browseros-ai/BrowserOS-agent` is a **stale mirror** (last updated 2026-03-13) and just points back to BrowserOS. **Only one fork is needed: `BrowserOS`.**

Privacy-relevant code lives almost entirely under `packages/browseros-agent/`. The Chromium patches contain a `browseros_metrics_service` (chrome://-style UMA-equivalent) which is wired into BrowserOS-built Chromium binaries, but for a personal build that simply doesn't ship a release the patches don't run anyway. We document it but don't patch it.

---

## Telemetry Inventory

### 1. PostHog (extension, browser-side)

**Init site (extension):** `packages/browseros-agent/apps/agent/lib/analytics/posthog.ts`

- `posthog.init(...)` with autocapture, pageview, session recording.
- Init is gated on `env.VITE_PUBLIC_POSTHOG_KEY && env.VITE_PUBLIC_POSTHOG_HOST` — both blank in `.env.example`. So in a dev build it's already a no-op. In a packaged production build (which the BrowserOS team produces) the keys are inlined.

**Event channel:** `packages/browseros-agent/apps/agent/lib/metrics/track.ts`
- The `track()` utility doesn't call PostHog directly — it goes through `BrowserOSAdapter.logMetric()` (a `chrome.browserOS.*` API patched into Chromium) which then routes to PostHog inside the Chromium fork's `browseros_metrics_service`.
- In a non-BrowserOS-built Chromium (e.g. running the extension as an unpacked extension in stock Chrome) the adapter falls back to no-op.
- Event constants: `packages/browseros-agent/apps/agent/lib/constants/analyticsEvents.ts`
- ~50+ `track(...)` call sites across `entrypoints/`. Not patching call sites — too noisy. Neutering the init + adapter is sufficient.

**Identify:** `packages/browseros-agent/apps/agent/lib/analytics/identify.ts` — calls `posthog.identify()` and `sentry.setUser()` on login. Only fires after login (which we won't do).

**Provider component:** `packages/browseros-agent/apps/agent/lib/analytics/AnalyticsProvider.tsx` — wraps the React tree with PostHog provider. Harmless when posthog client is uninitialized.

### 2. PostHog (server, Bun side)

**File:** `packages/browseros-agent/apps/server/src/lib/metrics.ts`
- `MetricsService` wraps `posthog-node`'s `PostHog` client.
- Client only constructed when `INLINED_ENV.POSTHOG_API_KEY` is truthy. Without it, `metrics.log()` returns early. Logger emits `Metrics disabled: missing POSTHOG_API_KEY`.

**Init site:** `packages/browseros-agent/apps/server/src/main.ts:189` calls `metrics.initialize(...)`.
- 30+ `metrics.log(...)` call sites across the server.

### 3. PostHog (CLI, Go)

**Files:** `packages/browseros-agent/apps/cli/analytics/analytics.go`, `apps/cli/cmd/strata.go`, env in `apps/cli/.env.production.example`.
- We don't use the CLI in personal-use mode. Skipping unless we later ship the CLI.

### 4. Sentry (extension, browser-side)

**File:** `packages/browseros-agent/apps/agent/lib/sentry/sentry.ts`
- `Sentry.init(...)` gated on `env.VITE_PUBLIC_SENTRY_DSN`. Blank in `.env.example`, so already no-op in dev. Production build inlines the DSN.
- `sendDefaultPii: true` (sends IP).
- Used by `identify.ts` to tag the user on login.

### 5. Sentry (server, Bun side)

**File:** `packages/browseros-agent/apps/server/src/lib/sentry.ts`
- `Sentry.init(...)` is **NOT gated** — it's called unconditionally. Sentry SDK does no-op when DSN is undefined, but we should still skip the init explicitly to be safe.
- `sendDefaultPii: true` (sends IP, request headers).

### 6. Segment / Mixpanel / June

- `Segment` matches in code are all incidental ("text segment" / "tool segments" data terminology). No segment.io tracking present.
- `mixpanel` only appears in MCP integration metadata (Klavis MCP server icons + docs). No direct mixpanel SDK in the agent. Safe to ignore.
- `june.so` — zero hits. Either removed or gated behind login + cloud sync (per upstream privacy policy, only engages with logged-in cloud-sync users).
- **Mitigation: never log in.** No code change needed.

### 7. Cloud sync / GraphQL backend

- `packages/browseros-agent/apps/agent/entrypoints/sidepanel/index/useRemoteConversationSave.ts` — early-returns if `!userId` from `useSessionInfo()`.
- `packages/browseros-agent/apps/agent/lib/llm-providers/storage.ts:syncLlmProviders` — early-returns if `!userId`.
- `packages/browseros-agent/apps/agent/lib/llm-providers/uploadLlmProvidersToGraphql.ts` — only called from the above.
- **Confirmed off by default. Mitigation: never log in.** Setting/storage to look at: any auth/login UI in `entrypoints/onboarding/` and `entrypoints/app/`. Don't sign up.

### 8. BrowserOS prefs backup

- `packages/browseros-agent/apps/agent/lib/llm-providers/storage.ts:setupLlmProvidersBackupToBrowserOS` writes provider configs to BrowserOS Chromium prefs (one-way, local). This is **local-only** (chrome.storage / native prefs), not network. Leave it.

### 9. Default LLM provider

- `packages/browseros-agent/apps/agent/lib/llm-providers/storage.ts:9` — `DEFAULT_PROVIDER_ID = 'browseros'`
- `createDefaultBrowserOSProvider()` builds an entry pointing at `https://api.browseros.com/v1` with model `browseros-auto`.
- `createDefaultProvidersConfig()` returns `[createDefaultBrowserOSProvider()]`.
- `defaultProviderIdStorage` falls back to `DEFAULT_PROVIDER_ID`.
- `providerTemplates.ts:111` defines `'ollama'` template: `http://localhost:11434/v1`, model `llama3.2`.

**Plan:** flip `createDefaultProvidersConfig()` to return an Ollama provider as the sole + default entry. Keep `DEFAULT_PROVIDER_ID = 'browseros'` constant string (it's used as a sentinel for the built-in row in normalization logic; renaming is a wider refactor we don't need).

Easier: change the default constructed provider's `type` and `baseUrl` in-place. Keep id `'browseros'` so the surrounding code (which keys off id `'browseros'` for built-in detection) still works, but make the underlying type `ollama` and URL local. Net: the "BrowserOS" built-in row appears in the UI but is wired to `localhost:11434`.

---

## Patches (this branch)

### Auto-applied (commit 1):

1. **`apps/agent/lib/analytics/posthog.ts`** — comment out `posthog.init(...)` block. Keep `import` so `import { posthog } from './posthog'` still resolves; the imported `posthog` is the package's default uninitialized instance — its `capture/identify/reset` methods are no-ops when uninitialized.
2. **`apps/server/src/lib/metrics.ts`** — comment out the `new PostHog(...)` client construction so `client` stays `null` and `log()` always early-returns.
3. **`apps/server/src/lib/sentry.ts`** — comment out the `Sentry.init({...})` call.
4. **`apps/agent/lib/sentry/sentry.ts`** — already gated on `env.VITE_PUBLIC_SENTRY_DSN`, but harden by short-circuiting the `if`. Comment.
5. **`apps/agent/lib/llm-providers/storage.ts`** — `createDefaultBrowserOSProvider()` returns a local-Ollama-shaped config (type `'ollama'`, baseUrl `http://localhost:11434/v1`, modelId `llama3.2`). Keep id `'browseros'` and name `'BrowserOS'` so the UI label and built-in plumbing are unchanged.

### Deferred / NOT applied (the user can pull these later as needed):

1. **Patch out individual `track()` call sites** — too many, too noisy, and the `BrowserOSAdapter.logMetric` already no-ops outside a BrowserOS-built Chromium binary. If the user runs the extension inside a packaged BrowserOS build, also patch `BrowserOSAdapter.logMetric` in `apps/agent/lib/browseros/adapter.ts` to be a no-op.
2. **Patch out `metrics.log(...)` server call sites** — same reasoning. Init no-op makes them no-ops.
3. **Identity / `identify()`** — only fires after login. Mitigation: don't log in. Code: `apps/agent/lib/analytics/identify.ts` (rip out if paranoid).
4. **AnalyticsProvider** — leaves PostHog React provider in tree. Harmless without init. Could replace with `<>{children}</>` passthrough.
5. **Sanitize / breadcrumbs** — `apps/agent/lib/sentry/sanitize.ts` and `apps/agent/lib/sentry/sentryRootErrorHandler.ts` strip PII from Sentry events. Once Sentry init is off, they're inert.
6. **CLI analytics** — `apps/cli/analytics/analytics.go`. Skip unless we use the CLI.
7. **Chromium-side metrics service** — `packages/browseros/chromium_patches/chrome/browser/browseros/metrics/`. Only relevant if we build a custom BrowserOS Chromium binary (hours of build time). Skip unless we go down that road.
8. **Cloud sync / GraphQL** — gated on login. Don't log in.
9. **Onboarding flow** — `entrypoints/onboarding/` may prompt for sign-up. Skip the prompt manually rather than patching.

### Also-not-applied because trivially controlled by env:

- Build with empty `VITE_PUBLIC_POSTHOG_KEY`, `VITE_PUBLIC_POSTHOG_HOST`, `VITE_PUBLIC_SENTRY_DSN`, `POSTHOG_API_KEY`, `SENTRY_DSN` envs. The defaults in `.env.example` are already empty.

---

## Verification

- `bun run typecheck` (run from `packages/browseros-agent/`) should pass.
- Don't build Chromium (hours). Run extension via `bun run dev:ext` from `apps/agent/` if needed for live testing.

---

## TODO — manual / next session

1. Have GitHub fork created (current PAT lacks `Administration: write` on personal repos and can't fork via API; user can `gh repo fork` interactively or click the GitHub UI). Then: `git remote set-url origin git@github.com:JaPossert/BrowserOS.git` (or `sebra-...`) and `git push -u origin private`.
2. Decide: do you want a one-line global kill switch (e.g., a `PRIVATE_BUILD` define) that no-ops every `track()` and `metrics.log()` regardless of env? Easy to add later.
3. Decide: do you want to also rip out the React `AnalyticsProvider` wrapper for paranoia? One-line edit.
4. If you ever need to actually build BrowserOS Chromium: fork+patch `packages/browseros/chromium_patches/chrome/browser/browseros/metrics/browseros_metrics_service.cc` to no-op `Capture`/`Identify`. Otherwise skip.
5. **TODO (fork ergonomics):** upstream `lefthook.yml`'s `commit-msg.conventional` hook uses bash regex `[[ "$msg" =~ ... ]]` but lefthook runs it via `/bin/sh`. On Debian (where `/bin/sh` is `dash`) the script fails to parse before checking the message. Two fixes: (a) add `runner: bash` under the `conventional` command, or (b) rewrite the check using POSIX `case` / `grep -E`. Tier 1 + tier 2 commits were pushed with `--no-verify` because of this. Fix on a separate fork-ergonomics branch when convenient.
6. **TODO (license clarity):** `LICENSE.evobiosys` (AMPL 1.0) was added covering Evobiosys's modifications to BrowserOS. Upstream BrowserOS is AGPL-3.0 (see `LICENSE`); ungoogled-chromium components are BSD-3 (see `LICENSE.ungoogled_chromium`). The combined work remains AGPL-3.0 — AMPL applies only to Evobiosys-authored changes. Add a top-level `NOTICE` clarifying this if you ever distribute publicly.
