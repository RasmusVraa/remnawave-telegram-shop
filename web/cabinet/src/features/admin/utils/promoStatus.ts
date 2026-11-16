export type PromoDisplayStatus = 'active' | 'inactive' | 'expired' | 'exhausted'

/** Вкладка списка, в которую попадает промокод (так же считает бэкенд, §6.1 спеки). */
export type PromoListBucket = 'active' | 'ended' | 'off'

export function isPromoExpired(validUntil?: string | null, now = Date.now()): boolean {
  if (!validUntil) return false
  const ts = Date.parse(validUntil)
  return Number.isFinite(ts) && ts < now
}

/**
 * Есть ли лимит активаций. Ноль бэкенд считает безлимитом (promo/service.go:
 * `max == nil || *max == 0`), а старая форма правки умела его сохранить, —
 * такой код не «исчерпан».
 */
export function hasPromoUseLimit(maxUses?: number | null): maxUses is number {
  return maxUses != null && maxUses > 0
}

export function resolvePromoDisplayStatus(promo: {
  active: boolean
  valid_until?: string | null
  max_uses?: number | null
  uses_count?: number
}): PromoDisplayStatus {
  if (isPromoExpired(promo.valid_until)) return 'expired'
  if (!promo.active) return 'inactive'
  if (hasPromoUseLimit(promo.max_uses) && (promo.uses_count ?? 0) >= promo.max_uses) return 'exhausted'
  return 'active'
}

export function promoListBucket(promo: Parameters<typeof resolvePromoDisplayStatus>[0]): PromoListBucket {
  const status = resolvePromoDisplayStatus(promo)
  if (status === 'active') return 'active'
  if (status === 'inactive') return 'off'
  return 'ended'
}
