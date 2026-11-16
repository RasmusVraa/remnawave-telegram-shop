import { getTelegramInitData } from '@/lib/utils'

/**
 * Вкладка под страницу оплаты, открытая заранее — синхронно в обработчике клика.
 *
 * Safari (iOS и macOS) и Firefox пускают window.open только в пределах жеста
 * пользователя: после `await api.checkout(...)` жест уже «истёк», и попап молча
 * блокируется. Chrome держит жест ~5 секунд, поэтому на Android всё работало.
 *
 * Поэтому: открываем пустую вкладку до запроса, а после ответа ведём её на
 * payment_url. Если вкладку открыть не дали — уходим на оплату в текущей вкладке
 * (return_url провайдера вернёт на /payment/status/:id).
 *
 * Telegram Mini App: openLink часто молчит. Делаем попытку открыть банк и
 * оставляем WebApp на /payment/status — там авто-редирект и кнопка «Открыть оплату».
 */
export interface PaymentWindow {
  /** Ведёт на оплату. true — текущая страница остаётся (статус/попап), false — ушли на оплату здесь. */
  go(url: string): boolean
  /** Закрывает заранее открытую вкладку (ошибка запроса). */
  cancel(): void
}

export type OpenPaymentOptions = {
  /**
   * Если внешнее окно не открылось — уйти на оплату в этой же вкладке.
   * Для авто-редиректа со статуса лучше false: иначе кнопка-запасник не успеет показаться.
   */
  allowSameTab?: boolean
}

function isTelegramMiniApp(): boolean {
  return getTelegramInitData().length > 0
}

/** Попытка открыть оплату снаружи (openLink / window.open), без ухода с текущей вкладки. */
export function tryOpenPaymentExternal(url: string): boolean {
  const trimmed = url.trim()
  if (!trimmed) return false

  const tg = window.Telegram?.WebApp
  if (isTelegramMiniApp() && typeof tg?.openLink === 'function') {
    try {
      tg.openLink(trimmed, { try_instant_view: false })
    } catch {
      /* дальше window.open */
    }
  }

  try {
    const w = window.open(trimmed, '_blank')
    if (w) {
      try {
        w.opener = null
      } catch {
        /* ignore */
      }
      return true
    }
  } catch {
    /* ignore */
  }
  return false
}

/**
 * Открыть URL оплаты.
 * Telegram: openLink → window.open → (опционально) текущая вкладка.
 * Браузер: window.open → текущая вкладка.
 */
export function openPaymentUrl(url: string, opts: OpenPaymentOptions = {}): void {
  const allowSameTab = opts.allowSameTab !== false
  if (tryOpenPaymentExternal(url)) return
  if (allowSameTab) {
    const trimmed = url.trim()
    if (trimmed) window.location.assign(trimmed)
  }
}

export function reservePaymentWindow(): PaymentWindow {
  if (isTelegramMiniApp()) {
    return {
      go(url) {
        // Пытаемся открыть банк сразу (жест ещё «живой» только частично —
        // openLink Telegram обычно принимает и без него).
        tryOpenPaymentExternal(url)
        // Всегда оставляем Mini App на статусе: авто-редирект + кнопка.
        return true
      },
      cancel() {},
    }
  }

  let w: Window | null = null
  try {
    // Без 'noopener': с ним window.open всегда возвращает null и вкладкой не управлять.
    w = window.open('', '_blank')
  } catch {
    w = null
  }
  if (w) {
    try {
      w.opener = null
      w.document.title = '…'
      w.document.body.style.cssText =
        'margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;' +
        'font:15px system-ui,-apple-system,sans-serif;color:#8a94a6;background:#0f1520'
      w.document.body.textContent = document.documentElement.lang === 'en' ? 'Loading…' : 'Загрузка…'
    } catch {
      /* оформление заглушки не критично */
    }
  }

  return {
    go(url) {
      if (w && !w.closed) {
        try {
          w.location.replace(url)
          return true
        } catch {
          /* упадём в текущую вкладку */
        }
      }
      window.location.assign(url)
      return false
    },
    cancel() {
      if (w && !w.closed) w.close()
    },
  }
}
