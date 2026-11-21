import remarkGfm from 'remark-gfm'
import type { Plugin } from 'unified'

/**
 * remark-gfm для браузеров без lookbehind в регулярках.
 *
 * Трансформ автоссылок из mdast-util-gfm-autolink-literal собирает
 * `new RegExp("(?<=^|\\s|…)")`. Safari до iOS 16.4 не знает lookbehind и бросает
 * SyntaxError прямо при рендере Markdown, поэтому на старых айфонах падали
 * покупка тарифа (описание) и подключение устройства (инструкция).
 *
 * Сами автоссылки остаются: http(s)://, www. и e-mail разбирает токенизатор
 * micromark, трансформ лишь дополнительно обходит дерево. Отключаем только его.
 */
type Ext = { transforms?: unknown } | Ext[]

/** gfmFromMarkdown() отдаёт массив расширений, поэтому обходим вложенность. */
function stripTransforms(ext: Ext) {
  if (Array.isArray(ext)) ext.forEach(stripTransforms)
  else delete ext.transforms
}

const remarkGfmCompat: Plugin<[], import('mdast').Root> = function () {
  const data = this.data() as { fromMarkdownExtensions?: Ext[] }
  const before = data.fromMarkdownExtensions?.length ?? 0
  ;(remarkGfm as unknown as (this: unknown) => void).call(this)
  const added = data.fromMarkdownExtensions?.slice(before) ?? []
  added.forEach(stripTransforms)
}

export default remarkGfmCompat
