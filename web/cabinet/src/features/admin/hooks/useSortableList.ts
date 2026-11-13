import { useEffect, useLayoutEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from 'react'

import { hapticImpact, hapticSelection } from '@/lib/haptic'

/**
 * Перетаскивание строк списка за ручку — как на странице статуса серверов.
 * Строка поднимается и идёт за курсором, соседи плавно расступаются, поэтому
 * видно, куда она встанет. Порядок хранит вызывающий: на каждый сдвиг хук
 * отдаёт новый массив id в onReorder.
 */
export function useSortableList(ids: string[], onReorder: (next: string[]) => void) {
  const listRef = useRef<HTMLUListElement>(null)
  const rowRefs = useRef(new Map<string, HTMLLIElement>())
  // Где строка стояла в прошлый раз (offsetTop не зависит от transform): по разнице играем переезд.
  const prevTops = useRef(new Map<string, number>())
  const pointerY = useRef(0)
  // Насколько ниже верхнего края строки пользователь взялся за ручку.
  const grabOffset = useRef(0)
  const [dragId, setDragId] = useState<string | null>(null)

  function placeDragged() {
    const el = dragId ? rowRefs.current.get(dragId) : undefined
    const ul = listRef.current
    if (!el || !ul) return
    const layoutTop = ul.getBoundingClientRect().top + el.offsetTop
    const dy = pointerY.current - grabOffset.current - layoutTop
    el.style.transition = 'none'
    el.style.transform = `translateY(${dy}px) scale(1.02)`
  }

  function onPointerMove(clientY: number) {
    if (!dragId || !listRef.current) return
    pointerY.current = clientY
    placeDragged()
    const dragged = rowRefs.current.get(dragId)
    const from = ids.indexOf(dragId)
    if (!dragged || from < 0) return
    const ulTop = listRef.current.getBoundingClientRect().top
    const draggedCenter = clientY - grabOffset.current + dragged.offsetHeight / 2
    const to = ids.filter((id) => {
      if (id === dragId) return false
      const el = rowRefs.current.get(id)
      return el ? ulTop + el.offsetTop + el.offsetHeight / 2 < draggedCenter : false
    }).length
    if (to === from) return
    const next = ids.filter((id) => id !== dragId)
    next.splice(to, 0, dragId)
    // Тик на каждый шаг строки — как у перетаскивания в списках iOS.
    hapticSelection()
    onReorder(next)
  }

  function endDrag() {
    if (!dragId) return
    const el = rowRefs.current.get(dragId)
    if (el) {
      el.style.transition = 'transform 200ms cubic-bezier(0.2, 0.8, 0.2, 1), box-shadow 200ms, border-color 200ms'
      el.style.transform = ''
    }
    setDragId(null)
  }

  // Анимация переезда: после смены порядка соседи «отъезжают» от старого места к новому.
  useLayoutEffect(() => {
    const reduce = typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
    rowRefs.current.forEach((el, id) => {
      const top = el.offsetTop
      const prev = prevTops.current.get(id)
      if (!reduce && prev != null && prev !== top && id !== dragId) {
        el.style.transition = 'none'
        el.style.transform = `translateY(${prev - top}px)`
        void el.getBoundingClientRect()
        el.style.transition = 'transform 200ms cubic-bezier(0.2, 0.8, 0.2, 1)'
        el.style.transform = ''
      }
      prevTops.current.set(id, top)
    })
    placeDragged()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ids.join(','), dragId])

  // Курсор «схватил» на всём экране, пока тянем.
  useEffect(() => {
    if (!dragId) return
    const prev = document.body.style.cursor
    document.body.style.cursor = 'grabbing'
    return () => {
      document.body.style.cursor = prev
    }
  }, [dragId])

  function move(id: string, dir: -1 | 1) {
    const from = ids.indexOf(id)
    const to = from + dir
    if (from < 0 || to < 0 || to >= ids.length) return
    const next = [...ids]
    next.splice(from, 1)
    next.splice(to, 0, id)
    onReorder(next)
  }

  return {
    dragId,
    listProps: {
      ref: listRef,
      onPointerMove: (e: ReactPointerEvent) => onPointerMove(e.clientY),
      onPointerUp: endDrag,
      onPointerCancel: endDrag,
    },
    itemRef: (id: string) => (el: HTMLLIElement | null) => {
      if (el) rowRefs.current.set(id, el)
      else rowRefs.current.delete(id)
    },
    handleProps: (id: string) => ({
      onPointerDown: (e: ReactPointerEvent<HTMLButtonElement>) => {
        e.preventDefault()
        e.currentTarget.setPointerCapture(e.pointerId)
        const el = rowRefs.current.get(id)
        pointerY.current = e.clientY
        grabOffset.current = el ? e.clientY - el.getBoundingClientRect().top : 0
        hapticImpact('medium')
        setDragId(id)
      },
      onKeyDown: (e: React.KeyboardEvent) => {
        if (e.key === 'ArrowUp' || e.key === 'ArrowDown') {
          e.preventDefault()
          move(id, e.key === 'ArrowUp' ? -1 : 1)
        }
      },
    }),
  }
}
