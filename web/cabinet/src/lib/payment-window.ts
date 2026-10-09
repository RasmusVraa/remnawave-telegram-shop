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
 */
export interface PaymentWindow {
  /** Ведёт на оплату. true — открыто в отдельной вкладке/Telegram, текущая страница остаётся. */
  go(url: string): boolean
  /** Закрывает заранее открытую вкладку (ошибка запроса). */
  cancel(): void
}

export function reservePaymentWindow(): PaymentWindow {
  const tg = window.Telegram?.WebApp
  if (getTelegramInitData().length > 0 && tg?.openLink) {
    return {
      go(url) {
        tg.openLink?.(url, { try_instant_view: false })
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
