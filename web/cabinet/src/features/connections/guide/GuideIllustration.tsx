import { useState, type CSSProperties, type ReactNode } from 'react'
import { Check, Download, Power } from 'lucide-react'

import { cn } from '@/lib/utils'
import { AppTile } from './glyphs'
import type { AppGuide } from './types'

export type SceneKind = 'install' | 'add' | 'connect' | 'done'
export type SceneFreeze = 'none' | 'start' | 'end'

type Props = {
  kind: SceneKind
  shape: 'phone' | 'laptop'
  app: Pick<AppGuide, 'id' | 'name'>
  brandName: string
  brandLogoUrl: string
  labels: { add: string; added: string; off: string; on: string }
  freeze?: SceneFreeze
  className?: string
}

const CONFETTI = ['#38bdf8', '#a78bfa', '#4ade80', '#fbbf24', '#f472b6', '#38bdf8', '#4ade80', '#a78bfa', '#fbbf24', '#f472b6', '#38bdf8', '#4ade80', '#a78bfa', '#fbbf24', '#f472b6', '#4ade80']

/** Логотип сервиса в карточке подписки: logoUrl, иначе первая буква названия. */
function BrandMark({ name, logoUrl }: { name: string; logoUrl: string }) {
  const [failed, setFailed] = useState(false)
  return (
    <span className="cg-sub-logo">
      {logoUrl && !failed ? (
        <img src={logoUrl} alt="" onError={() => setFailed(true)} />
      ) : (
        (name.trim()[0] || 'V').toUpperCase()
      )}
    </span>
  )
}

/** Анимированная сцена шага. Чистая разметка — движение целиком в connections-guide.css. */
export function GuideIllustration({ kind, shape, app, brandName, brandLogoUrl, labels, freeze = 'none', className }: Props) {
  const freezeAttr = freeze === 'none' ? undefined : freeze

  if (kind === 'done') {
    return (
      <div className={cn('cg-ill', className)} data-freeze={freezeAttr} aria-hidden>
        <span className="cg-ring" />
        <span className="cg-ring" />
        <div className="cg-confetti">
          {CONFETTI.map((c, i) => {
            const a = (i / CONFETTI.length) * Math.PI * 2
            const r = 72 + (i % 3) * 22
            return (
              <i
                key={i}
                style={
                  {
                    '--c': c,
                    '--x': `${Math.round(Math.cos(a) * r)}px`,
                    '--y': `${Math.round(Math.sin(a) * r)}px`,
                    '--dl': `${(i % 4) * 0.05}s`,
                  } as CSSProperties
                }
              />
            )
          })}
        </div>
        <div className="cg-done-ic">
          <Check size={36} strokeWidth={2.5} />
        </div>
      </div>
    )
  }

  let screen: ReactNode
  if (kind === 'install') {
    screen = (
      <>
        <AppTile app={app} className="cg-app">
          <span className="cg-okb">
            <Check size={11} strokeWidth={3} />
          </span>
        </AppTile>
        <div className="cg-bar">
          <i />
        </div>
      </>
    )
  } else if (kind === 'add') {
    screen = (
      <>
        <span className="cg-mini-btn">{labels.add}</span>
        <span className="cg-tap" />
        <div className="cg-sub">
          <BrandMark name={brandName || app.name} logoUrl={brandLogoUrl} />
          <span className="cg-sub-name">
            <b>{brandName || 'VPN'}</b>
            <small>{labels.added}</small>
          </span>
          <span className="cg-sub-ck">
            <Check size={12} strokeWidth={3} />
          </span>
        </div>
      </>
    )
  } else {
    screen = (
      <>
        <div className="cg-pw">
          <Power size={24} />
          <span className="cg-tap" />
        </div>
        <div className="cg-pw-label">
          <span className="cg-pw-off">{labels.off}</span>
          <span className="cg-pw-on">{labels.on}</span>
        </div>
      </>
    )
  }

  return (
    <div className={cn('cg-ill', kind === 'install' && 'cg-dl', kind === 'add' && 'cg-add', className)} data-freeze={freezeAttr} aria-hidden>
      <div className={cn('cg-dev', shape === 'phone' ? 'cg-dev--phone' : 'cg-dev--laptop')}>
        {kind === 'install' ? (
          <span className="cg-arrow">
            <Download size={22} />
          </span>
        ) : null}
        <div className="cg-screen">{screen}</div>
      </div>
    </div>
  )
}
