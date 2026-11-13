import { normalizeTariffsResponse, type TariffsResponse } from '@/lib/api'

/**
 * Мок витрины тарифов для локального просмотра лендинга без запущенного бота.
 *
 * Включается через localStorage и работает ТОЛЬКО в dev-сборке
 * (import.meta.env.DEV) — в проде ветка вырезается бандлером.
 *
 *   localStorage.setItem('landing_mock_tariffs', 'classic')  // периоды подписки
 *   localStorage.setItem('landing_mock_tariffs', 'plans')    // несколько тарифов
 *   localStorage.setItem('landing_mock_tariffs', '<json>')   // свой ответ GET /tariffs
 *   localStorage.removeItem('landing_mock_tariffs')          // выключить
 *
 * После изменения ключа страницу нужно перезагрузить.
 */

export const LANDING_MOCK_KEY = 'landing_mock_tariffs'

const GB = 1024 * 1024 * 1024

/** classic-режим: один тариф, карточки = периоды. Цены с базой 238.62 ₽/мес. */
const CLASSIC_RAW = {
  sales_mode: 'classic',
  currency: 'RUB',
  price_display: 'monthly',
  tariffs: [
    {
      id: 1,
      slug: 'base',
      name: 'Безлимитный',
      device_limit: 5,
      traffic_limit_bytes: 0,
      prices: [
        { months: 1, amount_rub: 238.62, monthly_base_rub: 238.62 },
        { months: 3, amount_rub: 644, monthly_base_rub: 238.62 },
        { months: 6, amount_rub: 1145, monthly_base_rub: 238.62 },
        { months: 12, amount_rub: 2004, monthly_base_rub: 238.62 },
      ],
    },
  ],
}

/** tariffs-режим: несколько тарифов, карточки = тарифы, цена за месяц. */
const PLANS_RAW = {
  sales_mode: 'tariffs',
  currency: 'RUB',
  price_display: 'monthly',
  tariffs: [
    {
      id: 1,
      slug: 'start',
      name: 'Старт',
      device_limit: 2,
      traffic_limit_bytes: 100 * GB,
      prices: [
        { months: 1, amount_rub: 149, monthly_base_rub: 149 },
        { months: 12, amount_rub: 1490, monthly_base_rub: 149 },
      ],
    },
    {
      id: 2,
      slug: 'pro',
      name: 'Про',
      device_limit: 5,
      traffic_limit_bytes: 0,
      prices: [
        { months: 1, amount_rub: 249, monthly_base_rub: 249 },
        { months: 12, amount_rub: 2490, monthly_base_rub: 249 },
      ],
    },
    {
      id: 3,
      slug: 'premium',
      name: 'Премиум',
      is_popular: true,
      device_limit: 10,
      traffic_limit_bytes: 0,
      prices: [
        { months: 1, amount_rub: 399, monthly_base_rub: 399 },
        { months: 12, amount_rub: 3990, monthly_base_rub: 399 },
      ],
    },
  ],
}

/**
 * Возвращает мок-витрину, если ключ выставлен, иначе null.
 * Любая ошибка разбора трактуется как «мока нет» — лендинг просто идёт в API.
 */
export function readLandingTariffsMock(): TariffsResponse | null {
  if (!import.meta.env.DEV) return null

  let raw: string | null = null
  try {
    raw = window.localStorage.getItem(LANDING_MOCK_KEY)
  } catch {
    return null
  }
  const value = raw?.trim()
  if (!value) return null

  if (value === 'classic' || value === '1') {
    return normalizeTariffsResponse(CLASSIC_RAW as never)
  }
  if (value === 'plans' || value === 'tariffs') {
    return normalizeTariffsResponse(PLANS_RAW as never)
  }

  try {
    return normalizeTariffsResponse(JSON.parse(value))
  } catch {
    console.warn(
      `[landing] localStorage.${LANDING_MOCK_KEY} не разобран как JSON. Допустимо: 'classic', 'plans' или ответ GET /tariffs.`,
    )
    return null
  }
}

/** Подсказка в консоли dev-сборки, чтобы мок не приходилось искать по коду. */
export function logLandingMockHint(): void {
  if (!import.meta.env.DEV) return
  console.info(
    `[landing] Мок тарифов: localStorage.setItem('${LANDING_MOCK_KEY}', 'classic' | 'plans') и перезагрузить страницу.`,
  )
}
