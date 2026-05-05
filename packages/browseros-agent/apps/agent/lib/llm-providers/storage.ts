import { storage } from '@wxt-dev/storage'
import { sessionStorage } from '@/lib/auth/sessionStorage'
import { getBrowserOSAdapter } from '@/lib/browseros/adapter'
import { BROWSEROS_PREFS } from '@/lib/browseros/prefs'
import type { LlmProviderConfig, LlmProvidersBackup } from './types'
import { uploadLlmProvidersToGraphql } from './uploadLlmProvidersToGraphql'

/** Default provider ID constant */
export const DEFAULT_PROVIDER_ID = 'browseros'
const DEFAULT_PROVIDER_NAME = 'BrowserOS'

/** Storage key for LLM providers array */
export const providersStorage = storage.defineItem<LlmProviderConfig[]>(
  'local:llm-providers',
  {
    version: 2,
    migrations: {
      2: (
        providers: LlmProviderConfig[] | null,
      ): LlmProviderConfig[] | null => {
        if (!providers) return providers
        return providers.map((provider) => {
          if (
            provider.id === DEFAULT_PROVIDER_ID &&
            provider.type === 'browseros'
          ) {
            return { ...provider, contextWindow: 200000 }
          }
          return provider
        })
      },
    },
  },
)

/** Backup providers to BrowserOS prefs (write-only, best-effort) */
async function backupToBrowserOS(backup: LlmProvidersBackup): Promise<void> {
  try {
    const adapter = getBrowserOSAdapter()
    await adapter.setPref(BROWSEROS_PREFS.PROVIDERS, JSON.stringify(backup))
  } catch {
    // BrowserOS API not available - ignore
  }
}

/**
 * Setup one-way sync of LLM providers to BrowserOS prefs
 * @public
 */
export function setupLlmProvidersBackupToBrowserOS(): () => void {
  const unsubscribe = providersStorage.watch(async (providers) => {
    if (providers) {
      const defaultProviderId = await defaultProviderIdStorage.getValue()
      await backupToBrowserOS({ defaultProviderId, providers })
    }
  })
  return unsubscribe
}

export async function syncLlmProviders(): Promise<void> {
  const providers = await providersStorage.getValue()
  if (!providers || providers.length === 0) return

  const session = await sessionStorage.getValue()
  const userId = session?.user?.id
  if (!userId) return

  await uploadLlmProvidersToGraphql(providers, userId)
}

/**
 * Setup one-way sync of LLM providers to GraphQL backend
 * Watches for storage changes and uploads non-sensitive provider data
 * @public
 */
export function setupLlmProvidersSyncToBackend(): () => void {
  syncLlmProviders().catch(() => {})

  const unsubscribe = providersStorage.watch(async () => {
    try {
      await syncLlmProviders()
    } catch {
      // Sync failed silently - will retry on next storage change
    }
  })
  return unsubscribe
}

/** Load providers from storage */
export async function loadProviders(): Promise<LlmProviderConfig[]> {
  const providers = (await providersStorage.getValue()) || []
  const normalizedProviders = normalizeProviderNames(providers)

  // Keep storage consistent so every consumer sees the same provider name.
  if (
    normalizedProviders.some((provider, index) => provider !== providers[index])
  ) {
    await providersStorage.setValue(normalizedProviders)
  }

  return normalizedProviders
}

/** Creates the default BrowserOS provider configuration */
export function createDefaultBrowserOSProvider(): LlmProviderConfig {
  const timestamp = Date.now()
  // privacy fork: built-in default points at local Ollama instead of the
  // hosted BrowserOS endpoint. Id/name kept so downstream "is this the
  // built-in row?" checks (which key off DEFAULT_PROVIDER_ID) still work.
  // To use: run `ollama pull llama3.2` (or any model) and `ollama serve`.
  return {
    id: DEFAULT_PROVIDER_ID,
    type: 'ollama',
    name: DEFAULT_PROVIDER_NAME,
    baseUrl: 'http://localhost:11434/v1',
    modelId: 'llama3.2',
    supportsImages: false,
    contextWindow: 128000,
    temperature: 0.2,
    createdAt: timestamp,
    updatedAt: timestamp,
  }
}

/** Creates the default providers configuration. Only call when storage is empty. */
export function createDefaultProvidersConfig(): LlmProviderConfig[] {
  return [createDefaultBrowserOSProvider()]
}

/**
 * Normalize built-in provider names back to "BrowserOS" (e.g. from "Kimi K2.5"
 * which was set during a previous partnership launch).
 */
function normalizeProviderNames(
  providers: LlmProviderConfig[],
): LlmProviderConfig[] {
  return providers.map((provider) => {
    if (
      provider.id === DEFAULT_PROVIDER_ID &&
      provider.type === 'browseros' &&
      provider.name !== DEFAULT_PROVIDER_NAME
    ) {
      return {
        ...provider,
        name: DEFAULT_PROVIDER_NAME,
      }
    }
    return provider
  })
}

/** Storage key for the default provider ID */
export const defaultProviderIdStorage = storage.defineItem<string>(
  'local:default-provider-id',
  {
    fallback: DEFAULT_PROVIDER_ID,
  },
)
