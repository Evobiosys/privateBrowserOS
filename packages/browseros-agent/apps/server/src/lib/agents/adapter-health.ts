/**
 * @license
 * Copyright 2025 BrowserOS
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */

import { exec } from 'node:child_process'
import { promisify } from 'node:util'
import { logger } from '../logger'
import type { AgentAdapter } from './agent-types'

const execAsync = promisify(exec)

export interface AdapterHealth {
  healthy: boolean
  /** Human-readable explanation when unhealthy; absent on success. */
  reason?: string
  /** Wall-clock ms when this probe completed. */
  checkedAt: number
}

interface CachedHealth extends AdapterHealth {
  expiresAt: number
}

/**
 * In-memory cache of adapter binary availability. Probed lazily on
 * first read and refreshed every `cacheTtlMs`. The probe is one
 * `<binary> --version` invocation per adapter with a hard 2s timeout
 * so a hung CLI doesn't block the listing endpoint.
 *
 * OpenClaw isn't probed here — its health derives from the gateway
 * lifecycle snapshot already exposed via `getGatewayStatus()`.
 */
export class AdapterHealthChecker {
  private readonly cache = new Map<AgentAdapter, CachedHealth>()
  private readonly cacheTtlMs: number
  private readonly probeTimeoutMs: number
  private readonly inflight = new Map<AgentAdapter, Promise<AdapterHealth>>()

  constructor(options: { cacheTtlMs?: number; probeTimeoutMs?: number } = {}) {
    this.cacheTtlMs = options.cacheTtlMs ?? 5 * 60 * 1000
    this.probeTimeoutMs = options.probeTimeoutMs ?? 2_000
  }

  async getHealth(adapter: AgentAdapter): Promise<AdapterHealth> {
    if (adapter === 'openclaw') {
      // OpenClaw health is derived from the gateway snapshot the
      // harness service already returns; the row component reads
      // that path. Surface a permissive default so the dot doesn't
      // spuriously light up red.
      return { healthy: true, checkedAt: Date.now() }
    }
    const now = Date.now()
    const cached = this.cache.get(adapter)
    if (cached && cached.expiresAt > now) return cached

    const inflight = this.inflight.get(adapter)
    if (inflight) return inflight

    const probe = this.runProbe(adapter)
      .then((result) => {
        const cacheEntry: CachedHealth = {
          ...result,
          expiresAt: Date.now() + this.cacheTtlMs,
        }
        this.cache.set(adapter, cacheEntry)
        return result
      })
      .finally(() => {
        this.inflight.delete(adapter)
      })
    this.inflight.set(adapter, probe)
    return probe
  }

  private async runProbe(adapter: AgentAdapter): Promise<AdapterHealth> {
    const command = ADAPTER_HEALTH_COMMANDS[adapter]
    if (!command) {
      return {
        healthy: false,
        reason: 'No health probe defined',
        checkedAt: Date.now(),
      }
    }
    try {
      await execAsync(command, { timeout: this.probeTimeoutMs })
      return { healthy: true, checkedAt: Date.now() }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      logger.debug('Adapter health probe failed', { adapter, error: message })
      return {
        healthy: false,
        reason: friendlyProbeFailure(adapter, message),
        checkedAt: Date.now(),
      }
    }
  }
}

/**
 * Probes are deliberately conservative — `--version` exits zero on
 * any installed CLI and won't trigger network calls or auth flows.
 */
const ADAPTER_HEALTH_COMMANDS: Partial<Record<AgentAdapter, string>> = {
  claude: 'claude --version',
  codex: 'codex --version',
}

function friendlyProbeFailure(adapter: AgentAdapter, raw: string): string {
  if (/command not found|not recognized|ENOENT/i.test(raw)) {
    return `${ADAPTER_HEALTH_COMMANDS[adapter]} failed: command not found`
  }
  if (/timed out|ETIMEDOUT/i.test(raw)) {
    return `${ADAPTER_HEALTH_COMMANDS[adapter]} did not respond within timeout`
  }
  return raw.split('\n')[0]?.slice(0, 200) ?? raw
}
