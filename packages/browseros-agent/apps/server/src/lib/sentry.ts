/**
 * @license
 * Copyright 2025 BrowserOS
 */

import { sanitizeEvent } from '@browseros/shared/sentry/sanitize'
import * as Sentry from '@sentry/bun'

import { INLINED_ENV } from '../env'
import { VERSION } from '../version'

const SENTRY_ENVIRONMENT = process.env.NODE_ENV || 'development'

// privacy fork: server-side Sentry init disabled. The Sentry namespace is
// still imported/exported so existing `Sentry.captureException(...)` call
// sites remain valid; without init they no-op.
// Sentry.init({
//   dsn: INLINED_ENV.SENTRY_DSN,
//   // Adds request headers and IP for users, for more info visit:
//   // https://docs.sentry.io/platforms/javascript/guides/bun/configuration/options/#sendDefaultPii
//   sendDefaultPii: true,
//   environment: SENTRY_ENVIRONMENT,
//   release: VERSION,
//
//   beforeSend(event) {
//     // Group tool execution errors by tool name instead of generic "execute"
//     const message = event.exception?.values?.[0]?.value ?? ''
//     if (message.startsWith('Internal error in ')) {
//       const toolName = message.match(/Internal error in (\S+):/)?.[1]
//       if (toolName) {
//         event.fingerprint = ['tool-execution', toolName]
//       }
//     }
//
//     return sanitizeEvent(event)
//   },
// })
void INLINED_ENV
void VERSION
void SENTRY_ENVIRONMENT
void sanitizeEvent

export { Sentry }
