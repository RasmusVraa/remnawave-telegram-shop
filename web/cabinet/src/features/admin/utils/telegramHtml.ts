/**
 * Разметка Telegram: сборка из редактора и разбор для предпросмотра.
 *
 * Текст рассылки уходит с `parse_mode=HTML`, поэтому набор тегов здесь — не наш
 * выбор, а список из документации Bot API. Всё, чего в нём нет, обязано стать
 * обычным текстом: Telegram отвергает сообщение целиком, если разметка не
 * разбирается, и одна опечатка роняет рассылку на всех получателях сразу.
 */

/** Теги-обёртки, которые понимает Telegram, по имени элемента в редакторе. */
const INLINE_TAGS: Record<string, string> = {
  B: 'b',
  STRONG: 'b',
  I: 'i',
  EM: 'i',
  U: 'u',
  INS: 'u',
  S: 's',
  STRIKE: 's',
  DEL: 's',
  CODE: 'code',
}

/** Node.TEXT_NODE / ELEMENT_NODE числами: модуль так проверяется вне браузера. */
const TEXT_NODE = 3
const ELEMENT_NODE = 1

/** Блочные обёртки браузера: своего значения не несут, дают перевод строки. */
const BLOCK_TAGS = new Set(['DIV', 'P', 'SECTION', 'ARTICLE', 'LI', 'H1', 'H2', 'H3', 'H4'])

/** Класс скрытого текста в редакторе; в разметке Telegram это `tg-spoiler`. */
export const SPOILER_CLASS = 'tg-spoiler'

/** Класс сворачиваемой цитаты; в разметке — `<blockquote expandable>`. */
export const EXPANDABLE_CLASS = 'is-expandable'

export function escapeHtml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

/**
 * Ссылки только http(s) и tg: — `javascript:` и `data:` не пропускаем.
 *
 * Проверка нужна в обе стороны: и когда админ вставляет ссылку в редактор,
 * и когда предпросмотр разворачивает разметку обратно в DOM.
 */
export function safeHref(raw: string): string | null {
  const url = raw.trim()
  if (/^https?:\/\//i.test(url) || /^tg:\/\//i.test(url)) return url
  return null
}

/**
 * Содержимое редактора → разметка Telegram.
 *
 * Текст экранируется, разрешённые обёртки переносятся, всё остальное
 * разворачивается: браузер по ходу правки создаёт свои `span` со стилями,
 * и отдавать их Telegram нельзя.
 *
 * Переводы строк собираются здесь же. Contenteditable хранит строки как `div`
 * и `br`, а Bot API ждёт обычный `\n`, поэтому блочные элементы превращаются
 * в перенос, а не в тег.
 */
export function serializeToTelegramHtml(root: Node): string {
  const out: string[] = []

  const endsWithNewline = () => {
    for (let i = out.length - 1; i >= 0; i--) {
      if (out[i] === '') continue
      return out[i].endsWith('\n')
    }
    return true
  }
  const breakLine = () => {
    if (!endsWithNewline()) out.push('\n')
  }

  const walk = (node: Node) => {
    node.childNodes.forEach((child) => {
      if (child.nodeType === TEXT_NODE) {
        out.push(escapeHtml(child.nodeValue ?? ''))
        return
      }
      if (child.nodeType !== ELEMENT_NODE) return

      const el = child as HTMLElement
      const tag = el.tagName

      if (tag === 'BR') {
        out.push('\n')
        return
      }

      if (tag === 'BLOCKQUOTE') {
        breakLine()
        // Вложенные цитаты Telegram не разбирает, поэтому внутренние
        // blockquote при обходе просто растворяются — остаётся текст.
        out.push(el.classList.contains(EXPANDABLE_CLASS) ? '<blockquote expandable>' : '<blockquote>')
        walk(el)
        out.push('</blockquote>\n')
        return
      }

      if (tag === 'A') {
        const href = safeHref(el.getAttribute('href') ?? '')
        if (!href) {
          walk(el)
          return
        }
        out.push(`<a href="${escapeHtml(href)}">`)
        walk(el)
        out.push('</a>')
        return
      }

      if (tag === 'SPAN' && el.classList.contains(SPOILER_CLASS)) {
        out.push('<tg-spoiler>')
        walk(el)
        out.push('</tg-spoiler>')
        return
      }

      const inline = INLINE_TAGS[tag]
      if (inline) {
        out.push(`<${inline}>`)
        walk(el)
        out.push(`</${inline}>`)
        return
      }

      if (BLOCK_TAGS.has(tag)) {
        breakLine()
        walk(el)
        breakLine()
        return
      }

      // Неизвестный элемент (span со стилем от браузера, вставка из буфера)
      // разворачиваем: наружу должен уйти только его текст.
      walk(el)
    })
  }

  walk(root)
  return out.join('').replace(/\n{3,}/g, '\n\n').trim()
}

/** Теги без атрибутов, которые предпросмотр возвращает к жизни. */
const SIMPLE_TAGS = ['b', 'strong', 'i', 'em', 'u', 'ins', 's', 'strike', 'del', 'code', 'pre'] as const

/**
 * Разметка Telegram → безопасный HTML предпросмотра.
 *
 * Вставлять строку в разметку страницы как есть нельзя даже в админке:
 * `<script>` или `onerror` у картинки выполнятся в сессии администратора.
 * Поэтому сначала экранируем всё целиком, а потом возвращаем только теги из
 * белого списка — ровно так же, как это сделает Bot API.
 */
export function renderTelegramHtml(html: string): string {
  let out = escapeHtml(html)

  for (const tag of SIMPLE_TAGS) {
    out = out.replace(new RegExp(`&lt;${tag}&gt;`, 'gi'), `<${tag}>`)
    out = out.replace(new RegExp(`&lt;/${tag}&gt;`, 'gi'), `</${tag}>`)
  }

  out = out.replace(/&lt;tg-spoiler&gt;/gi, `<span class="${SPOILER_CLASS}">`)
  out = out.replace(/&lt;\/tg-spoiler&gt;/gi, '</span>')

  out = out.replace(/&lt;blockquote expandable&gt;/gi, `<blockquote class="${EXPANDABLE_CLASS}">`)
  out = out.replace(/&lt;blockquote&gt;/gi, '<blockquote>')
  out = out.replace(/&lt;\/blockquote&gt;/gi, '</blockquote>')

  out = out.replace(/&lt;a href=&quot;((?:(?!&quot;).)*)&quot;&gt;/gi, (match, href: string) => {
    // Адрес экранирован дважды: разметкой Telegram (&amp; в ссылке) и нами выше.
    const safe = safeHref(unescapeHtml(unescapeHtml(href)))
    return safe ? `<a href="${escapeHtml(safe)}" target="_blank" rel="noreferrer">` : match
  })
  out = out.replace(/&lt;\/a&gt;/gi, '</a>')

  // Сущности исходной разметки (&lt; &amp; …) экранирование выше превратило в
  // &amp;lt; — на экране вышло бы «&lt;» буквами. Возвращаем их последним
  // шагом, когда теги уже разобраны: так текст «&lt;b&gt;» останется текстом.
  out = out.replace(/&amp;(amp|lt|gt|quot|#\d+|#x[0-9a-f]+);/gi, '&$1;')

  // Переводы строк в предпросмотре рисуем сами: white-space их сохранит, но
  // внутри blockquote нужен именно <br>, иначе строка склеивается с текстом.
  return out.replace(/\n/g, '<br>')
}

function unescapeHtml(value: string): string {
  return value
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&amp;/g, '&')
}

/** Длина текста без разметки — по ней считается лимит Telegram. */
export function telegramTextLength(html: string): number {
  const withBreaks = html.replace(/<br\s*\/?>/gi, '\n')
  const text = withBreaks.replace(/<[^>]+>/g, '')
  const decoded = text
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&amp;/g, '&')
  return [...decoded.trim()].length
}
