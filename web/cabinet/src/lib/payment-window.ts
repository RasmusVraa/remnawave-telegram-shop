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
 * В Telegram WebApp `openLink` часто молча ничего не открывает, но мы всё равно
 * уходим на экран статуса — там кнопка «Открыть оплату» и повторный openPaymentUrl.
 */
export interface PaymentWindow {
  /** Ведёт на оплату. true — текущая страница остаётся (статус/попап), false — ушли на оплату здесь. */
  go(url: string): boolean
  /** Закрывает заранее открытую вкладку (ошибка запроса). */
  cancel(): void
}

/** Открыть URL оплаты: Telegram openLink → window.open → текущая вкладка. */
export function openPaymentUrl(url: string): void {
  const trimmed = url.trim()
  if (!trimmed) return

  const tg = window.Telegram?.WebApp
  if (getTelegramInitData().length > 0 && typeof tg?.openLink === 'function') {
    try {
      tg.openLink(trimmed, { try_instant_view: false })
    } catch {
      /* ниже — запасные пути */
    }
    // openLink не сообщает об успехе. Дублируем через window.open на жесте
    // пользователя (кнопка «Открыть оплату»); если и это блокируют — текущая вкладка.
    try {
      const w = window.open(trimmed, '_blank')
      if (w) {
        try {
          w.opener = null
        } catch {
          /* ignore */
        }
        return
      }
    } catch {
      /* ignore */
    }
    window.location.assign(trimmed)
    return
  }

  try {
    const w = window.open(trimmed, '_blank')
    if (w) {
      try {
        w.opener = null
      } catch {
        /* ignore */
      }
      return
    }
  } catch {
    /* ignore */
  }
  window.location.assign(trimmed)
}

export function reservePaymentWindow(): PaymentWindow {
  // Telegram Mini App: openLink часто молча не открывает банк. Надёжный путь —
  // уйти на оплату в этой же WebView; return_url вернёт на /payment/status/:id.
  // Запасная кнопка «Открыть оплату» на статусе всё равно нужна (рефреш / отмена).
  if (getTelegramInitData().length > 0) {
    return {
      go(url) {
        window.location.assign(url)
        return false
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
