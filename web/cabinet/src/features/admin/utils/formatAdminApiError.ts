import type { TFunction } from 'i18next'

import { ApiError } from '@/lib/api'

const BODY_KEY_MAP: Record<string, string> = {
  'panel not configured': 'admin.errors.panelUnavailable',
  'not found': 'admin.errors.notFound',
  'not implemented': 'admin.errors.notImplemented',
  'sync already in progress': 'admin.errors.syncInProgress',
  'recalc already in progress': 'admin.errors.recalcInProgress',
  'broadcast already in progress': 'admin.errors.broadcastInProgress',
  'test broadcast requires admin_telegram_id': 'admin.errors.testBroadcastNoAdmin',
  'no recipients': 'admin.errors.broadcastNoRecipients',
  'method not allowed': 'admin.errors.methodNotAllowed',
  'invalid id': 'admin.errors.invalidId',
  'no valid fields': 'admin.errors.noValidFields',
  'code and type are required': 'admin.errors.promoCodeRequired',
  'slug is required': 'admin.errors.slugRequired',
  'csrf: missing token': 'admin.errors.csrf',
  'csrf: token mismatch': 'admin.errors.csrf',
  'csrf: forbidden': 'admin.errors.csrf',
  'name is required': 'admin.errors.statusName',
  'name is too long': 'admin.errors.statusName',
  'country must be 2 letters': 'admin.errors.statusCountry',
  'address is not a public ip': 'admin.errors.statusAddress',
  'address is not a public host': 'admin.errors.statusAddress',
  'note is too long': 'admin.errors.statusNote',
  'too many targets': 'admin.errors.statusTooMany',
  'interval must be between 5 and 180': 'admin.errors.statusInterval',
  'world probes must be between 1 and 10': 'admin.errors.statusWorld',
  'russia probes must be between 1 and 30': 'admin.errors.statusRussia',
  'probe already running': 'admin.errors.statusProbeBusy',
  'no targets': 'admin.errors.statusProbeEmpty',
}

function normalizeBody(body: string): string {
  return body.trim().toLowerCase()
}

function mapBodyToKey(body: string): string | null {
  const norm = normalizeBody(body)
  if (!norm) return null
  for (const [needle, key] of Object.entries(BODY_KEY_MAP)) {
    if (norm.includes(needle)) return key
  }
  return null
}

export function formatAdminApiError(err: unknown, t: TFunction): string {
  if (err instanceof ApiError) {
    const bodyKey = mapBodyToKey(err.body)
    if (bodyKey) return t(bodyKey)

    switch (err.status) {
      case 400:
        return t('admin.errors.badRequest')
      case 401:
        return t('admin.errors.unauthorized')
      case 403:
        return t('admin.errors.forbidden')
      case 404:
        return t('admin.errors.notFound')
      case 409:
        return t('admin.errors.conflict')
      case 413:
        // Отдаёт nginx (client_max_body_size), а не бэкенд — тело будет HTML-страницей.
        return t('admin.errors.payloadTooLarge')
      case 429:
        return t('admin.errors.tooManyRequests')
      case 501:
        return t('admin.errors.notImplemented')
      case 503:
        return t('admin.errors.serviceUnavailable')
      default:
        if (err.status >= 500) return t('admin.errors.serverError')
        return t('admin.errors.requestFailed')
    }
  }
  return t('admin.errors.unknown')
}
