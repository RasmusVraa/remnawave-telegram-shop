import { isMobileUserAgent } from '@/lib/utils'
import { isTelegramMiniAppSession } from '@/lib/telegram-web-app-loader'

/**
 * «Поделиться» ссылкой — одна кнопка на все окружения кабинета.
 *
 * Долго кнопка была завязана на Web Share API и пряталась, когда
 * `navigator.share` отсутствует. В Mini App это означало «спрятана всегда»:
 * Telegram открывает мини-приложение в системном WebView, где Web Share API не
 * реализован, — то есть кнопки не было ровно там, где ссылкой делятся чаще
 * всего.
 *
 * Правильный путь внутри Telegram — не системный шит, а собственный метод
 * клиента: `WebApp.openTelegramLink('https://t.me/share/url?...')` сворачивает
 * мини-приложение и показывает родной выбор чата с уже подставленным текстом.
 *
 * Отсюда три ветки, именно в этом порядке:
 *
 *  1. Mini App (мобильный клиент, desktop-клиент, Telegram Web) — всегда
 *     `openTelegramLink`. Проверяем окружение, а не наличие `navigator.share`:
 *     если Telegram однажды добавит Web Share в свой WebView, пользователь
 *     получит системный шит вместо родного выбора чата — это шаг назад.
 *  2. Мобильный браузер — `navigator.share`: там системный шит настоящий, и в
 *     нём есть и Telegram, и всё остальное, куда человек может захотеть кинуть
 *     ссылку.
 *  3. Всё прочее, прежде всего десктопный браузер, — та же ссылка
 *     `t.me/share/url` в новой вкладке. На десктопе Web Share API формально
 *     есть, но показывает только зарегистрированные share target'ы, и список
 *     оказывался пустым; ссылка же отдаёт диалог Telegram Web или уже
 *     установленному клиенту.
 *
 * Функция никогда не бросает и возвращает признак «делёжка началась». `false` —
 * это и отказ пользователя, и заблокированная вкладка; вызывающий сам решает,
 * что показать. В кабинете это откат на копирование, чтобы нажатие не осталось
 * без результата.
 */
export interface ShareLinkInput {
  /** Сопроводительный текст без ссылки — её подставляет сама функция. */
  text: string
  url: string
}

/** Диалог Telegram «кому переслать» — и в клиенте, и на web.telegram.org. */
function telegramShareUrl(text: string, url: string): string {
  // encodeURIComponent, а не URLSearchParams: последний кодирует пробел как «+»,
  // и текст приезжает в поле ввода с плюсами вместо пробелов.
  return `https://t.me/share/url?url=${encodeURIComponent(url)}&text=${encodeURIComponent(text)}`
}

function openInNewTab(url: string): boolean {
  try {
    // Без 'noopener' в features: с ним window.open по спецификации всегда
    // возвращает null, и открытая вкладка выглядела бы заблокированной —
    // вызывающий каждый раз ещё и копировал ссылку. Связь рвём вручную.
    const w = window.open(url, '_blank')
    if (!w) return false
    try {
      w.opener = null
    } catch {
      /* вкладка уже открыта — это главное */
    }
    return true
  } catch {
    return false
  }
}

export async function shareLink({ text, url }: ShareLinkInput): Promise<boolean> {
  const link = url?.trim()
  if (!link) return false
  const message = text?.trim() ?? ''

  if (isTelegramMiniAppSession()) {
    const tg = window.Telegram?.WebApp
    if (tg && typeof tg.openTelegramLink === 'function') {
      try {
        tg.openTelegramLink(telegramShareUrl(message, link))
        return true
      } catch {
        // Метода нет в старом клиенте — уходим на общий путь ниже.
      }
    }
  }

  if (isMobileUserAgent() && typeof navigator.share === 'function') {
    try {
      // Ссылка идёт внутри текста, а не отдельным полем `url`: часть приложений
      // берёт из шита только текст, и ссылка терялась по дороге.
      await navigator.share({ text: `${message}\n${link}`.trim() })
      return true
    } catch {
      // Отмена или отказ. Открыть вкладку здесь уже нельзя: жест потрачен на
      // шит, и браузер заблокирует всплывающее окно — честно отдаём false.
      return false
    }
  }

  return openInNewTab(telegramShareUrl(message, link))
}
