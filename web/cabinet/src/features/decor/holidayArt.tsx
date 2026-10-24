import type { CSSProperties } from 'react'

/** Небо хеллоуина: паутина, луна-тыква и ведьма. Земля кабинета рисуется отдельно. */

export function SpiderCorner() {
  const rays = Array.from({ length: 9 }, (_, i) => {
    const angle = (Math.PI / 2) * (i / 8)
    return { x: Math.cos(angle) * 148, y: Math.sin(angle) * 148 }
  })
  return (
    <svg className="holiday-web" viewBox="0 0 170 170" aria-hidden>
      <g fill="none" stroke="#f4e7d4" strokeWidth="0.7" opacity="0.82">
        {rays.map((point, i) => (
          <line key={i} x1="6" y1="6" x2={6 + point.x} y2={6 + point.y} />
        ))}
        {[0.22, 0.4, 0.58, 0.76, 0.94].map((scale, ring) => (
          <path
            key={ring}
            d={rays
              .map((point, i) => {
                const x = (6 + point.x * scale).toFixed(1)
                const y = (6 + point.y * scale).toFixed(1)
                return `${i === 0 ? 'M' : 'L'}${x} ${y}`
              })
              .join(' ')}
          />
        ))}
      </g>
      <g transform="translate(92 70)">
        <ellipse cx="0" cy="7" rx="8" ry="10" fill="#140810" />
        <ellipse cx="0" cy="-1" rx="5.5" ry="5" fill="#24141e" />
        <circle cx="-1.7" cy="-2.2" r="1" fill="#ffb15a" />
        <circle cx="1.8" cy="-2.2" r="1" fill="#ffb15a" />
        <path
          d="M-5 1 C-18 -10 -26 -4 -34 2 M-5 4 C-20 2 -28 10 -36 8 M-4 8 C-16 16 -22 26 -18 34 M-1 10 C-8 20 -6 30 2 36"
          fill="none"
          stroke="#140810"
          strokeWidth="1.15"
          strokeLinecap="round"
        />
        <path
          d="M5 1 C18 -10 26 -4 34 2 M5 4 C20 2 28 10 36 8 M4 8 C16 16 22 26 18 34 M1 10 C8 20 6 30 -2 36"
          fill="none"
          stroke="#140810"
          strokeWidth="1.15"
          strokeLinecap="round"
        />
      </g>
    </svg>
  )
}

export function PumpkinMoon() {
  return (
    <svg className="holiday-moon" viewBox="0 0 210 240" aria-hidden>
      <defs>
        <radialGradient id="hw-moon-body" cx="42%" cy="38%" r="62%">
          <stop offset="0%" stopColor="#ffe7a3" />
          <stop offset="38%" stopColor="#ffb03a" />
          <stop offset="100%" stopColor="#c65310" />
        </radialGradient>
        <radialGradient id="hw-carve" cx="50%" cy="30%" r="75%">
          <stop offset="0%" stopColor="#fff6c8" />
          <stop offset="55%" stopColor="#ffd15a" />
          <stop offset="100%" stopColor="#ff7a18" />
        </radialGradient>
        <linearGradient id="hw-hat" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0%" stopColor="#4c3268" />
          <stop offset="100%" stopColor="#120810" />
        </linearGradient>
        <filter id="hw-face-glow" x="-60%" y="-60%" width="220%" height="220%">
          <feGaussianBlur stdDeviation="1.6" result="blur" />
          <feMerge>
            <feMergeNode in="blur" />
            <feMergeNode in="SourceGraphic" />
          </feMerge>
        </filter>
      </defs>
      <ellipse cx="112" cy="156" rx="78" ry="70" fill="#ff9a2e" opacity="0.2" />
      <ellipse cx="78" cy="158" rx="22" ry="52" fill="#c24e12" />
      <ellipse cx="146" cy="158" rx="22" ry="52" fill="#e07820" />
      <ellipse cx="112" cy="160" rx="32" ry="56" fill="url(#hw-moon-body)" />
      <ellipse cx="100" cy="158" rx="8" ry="48" fill="#e07018" opacity="0.35" />
      <ellipse cx="126" cy="156" rx="7" ry="46" fill="#ffd27a" opacity="0.22" />
      <path d="M118 78 C128 28 146 14 136 2 C164 26 170 62 150 82 Z" fill="url(#hw-hat)" />
      <ellipse cx="124" cy="84" rx="40" ry="9" fill="#1c1028" />
      <path d="M96 80 H150" stroke="#6d3cc8" strokeWidth="6" strokeLinecap="round" />
      <rect x="116" y="76" width="14" height="9" rx="1.5" fill="#e8c56b" />
      <rect x="120" y="78" width="6" height="5" fill="#8a6230" />
      <g fill="url(#hw-carve)">
        <path d="M82 132 H104 L93 154 Z" />
        <path d="M120 132 H142 L131 154 Z" />
        <path d="M106 156 H118 L112 170 Z" />
        <path d="M84 176 Q112 202 140 176 L134 182 Q112 194 90 182 Z" />
      </g>
    </svg>
  )
}

export function Witch() {
  return (
    <svg className="holiday-witch" viewBox="0 0 250 120" aria-hidden>
      <defs>
        <linearGradient id="hw-cloak" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="#4a2c72" />
          <stop offset="100%" stopColor="#140816" />
        </linearGradient>
        <linearGradient id="hw-hatg" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0%" stopColor="#5a3a78" />
          <stop offset="100%" stopColor="#120810" />
        </linearGradient>
      </defs>
      <path d="M28 84 H214" stroke="#8d5a32" strokeWidth="3.4" strokeLinecap="round" />
      <g stroke="#d59a48" strokeWidth="1.35" fill="none" strokeLinecap="round">
        <path d="M200 84 C216 64 236 66 246 78" />
        <path d="M202 84 C220 70 238 76 248 86" />
        <path d="M202 84 C220 86 238 94 248 102" />
        <path d="M198 84 C214 100 232 108 242 114" />
        <path d="M204 84 C214 104 224 112 230 116" />
      </g>
      <path d="M190 78 H208 L206 90 H192 Z" fill="#6a3a22" />
      <path d="M188 80 H204" stroke="#e8c56b" strokeWidth="2" />
      <path d="M72 52 C36 48 12 68 4 98 C28 84 50 80 76 76 C66 66 68 56 72 52Z" fill="#160a1c" />
      <path d="M84 48 C46 56 24 78 16 108 C48 92 70 86 92 80 C84 66 82 54 84 48Z" fill="url(#hw-cloak)" />
      <path d="M78 78 L62 108 L80 96 L94 114 L104 84 Z" fill="#2a163c" />
      <path d="M98 50 C90 64 88 78 94 88 L122 84 C128 68 122 54 114 48 Z" fill="#321c4c" />
      <path d="M108 58 C128 62 146 70 166 78" fill="none" stroke="#321c4c" strokeWidth="6" strokeLinecap="round" />
      <circle cx="168" cy="79" r="3.4" fill="#f0c8a8" />
      <circle cx="112" cy="44" r="12" fill="#f3c7a6" />
      <path d="M108 40 Q114 42 116 39" fill="none" stroke="#3a2014" strokeWidth="1.1" />
      <path d="M112 46 Q116 49 120 46" fill="none" stroke="#b46452" strokeWidth="0.9" />
      <path d="M102 40 C82 36 62 46 46 42 C68 50 84 46 100 50" fill="#1a0c14" />
      <path d="M100 34 C80 22 60 28 44 22 C66 34 84 30 102 38" fill="#241018" />
      <path d="M104 32 C90 16 70 12 56 4 C76 16 92 14 106 30" fill="#14080e" />
      <path d="M100 34 L128 2 C134 18 136 32 124 40 Z" fill="url(#hw-hatg)" />
      <ellipse cx="110" cy="36" rx="22" ry="6" fill="#1a1024" />
      <path d="M94 34 H126" stroke="#7a45d0" strokeWidth="3.4" strokeLinecap="round" />
      <rect x="110" y="31.5" width="8" height="6" rx="1" fill="#e8c56b" />
      <path d="M96 86 L92 104 H110 L106 86" fill="#12080e" />
      <path d="M110 100 H96" stroke="#e8c56b" strokeWidth="1.4" />
    </svg>
  )
}

export function Bat({ style }: { style?: CSSProperties }) {
  return (
    <svg className="holiday-bat" viewBox="0 0 100 42" style={style} aria-hidden>
      <path
        fill="#120810"
        d="M50 24
          C46 16 42 9 37 6
          C35 13 30 16 23 15
          C14 6 6 5 1 12
          C10 16 16 22 23 25
          C16 28 8 33 3 39
          C16 34 30 30 42 28
          C45 33 48 35 50 35
          C52 35 55 33 58 28
          C70 30 84 34 97 39
          C92 33 84 28 77 25
          C84 22 90 16 99 12
          C94 5 86 6 77 15
          C70 16 65 13 63 6
          C58 9 54 16 50 24Z"
      />
      <path d="M46 7 L43 1 L49 6 M54 7 L57 1 L51 6" fill="#120810" />
      <circle cx="46" cy="20" r="1.35" fill="#ffb15a" />
      <circle cx="54" cy="20" r="1.35" fill="#ffb15a" />
    </svg>
  )
}

export function HalloweenFrieze() {
  return (
    <div className="holiday-frieze">
      <BareTree />
      <PumpkinPatch />
      <HauntedHouse />
      <OrnateLantern />
    </div>
  )
}

function BareTree() {
  return (
    <svg className="holiday-figure holiday-figure--tall" viewBox="0 0 160 200" aria-hidden>
      <path d="M46 198 C50 150 58 118 64 108" fill="none" stroke="#2a160e" strokeWidth="11" strokeLinecap="round" />
      <path d="M62 112 H128" fill="none" stroke="#2a160e" strokeWidth="7" strokeLinecap="round" />
      <path d="M58 150 C40 142 26 146 16 134" fill="none" stroke="#3a2014" strokeWidth="4" strokeLinecap="round" />
      <path d="M66 132 C78 118 86 116 92 104" fill="none" stroke="#3a2014" strokeWidth="3.5" strokeLinecap="round" />
      <g transform="translate(96 112)">
        <path d="M-8 2 L-12 8 M-3 2 L-3 9 M7 2 L11 8" fill="none" stroke="#e7c27a" strokeWidth="1.6" strokeLinecap="round" />
        <ellipse cx="0" cy="-18" rx="16" ry="18" fill="#241420" />
        <path d="M-14 -8 C-4 2 8 2 16 -10 C6 -2 -6 -2 -14 -8Z" fill="#1a1018" />
        <ellipse cx="0" cy="-34" rx="13" ry="12" fill="#322030" />
        <path d="M-8 -42 L-12 -54 L-2 -42 M8 -42 L12 -54 L2 -42" fill="#241420" />
        <circle cx="-5" cy="-35" r="4.2" fill="#f4e2b0" />
        <circle cx="5" cy="-35" r="4.2" fill="#f4e2b0" />
        <circle cx="-5" cy="-35" r="1.7" fill="#1a1008" />
        <circle cx="5" cy="-35" r="1.7" fill="#1a1008" />
        <path d="M0 -31 L3 -26 L-2 -27 Z" fill="#e08a22" />
      </g>
    </svg>
  )
}

function PumpkinPatch() {
  return (
    <svg className="holiday-figure holiday-figure--short" viewBox="0 0 250 130" aria-hidden>
      <path d="M20 96 C40 70 30 48 18 40" fill="none" stroke="#3d6b32" strokeWidth="3" />
      <path d="M210 100 C196 72 214 50 230 38" fill="none" stroke="#3d6b32" strokeWidth="3" />
      <path d="M30 70 C48 60 40 48 34 42 C46 50 58 46 52 34" fill="#2f6a34" />
      <path d="M214 74 C198 62 208 50 214 40 C204 52 190 46 198 34" fill="#3d7a3c" />
      <CarvedPumpkin cx={62} cy={92} scale={0.85} />
      <CarvedPumpkin cx={124} cy={98} scale={1.05} />
      <CarvedPumpkin cx={186} cy={94} scale={0.78} />
    </svg>
  )
}

function CarvedPumpkin({ cx, cy, scale }: { cx: number; cy: number; scale: number }) {
  const s = scale
  return (
    <g transform={`translate(${cx} ${cy}) scale(${s})`}>
      <ellipse cx="-16" cy="2" rx="14" ry="28" fill="#c24a10" />
      <ellipse cx="16" cy="2" rx="14" ry="28" fill="#e87822" />
      <ellipse cx="0" cy="2" rx="16" ry="30" fill="#ff9a32" />
      <ellipse cx="-6" cy="0" rx="5" ry="26" fill="#e07018" opacity="0.55" />
      <ellipse cx="7" cy="-2" rx="4" ry="24" fill="#ffd27a" opacity="0.3" />
      <ellipse cx="-6" cy="-14" rx="8" ry="4" fill="#fff" opacity="0.18" />
      <path d="M0 -26 C2 -38 10 -40 12 -32" fill="none" stroke="#3e6b34" strokeWidth="3" strokeLinecap="round" />
      <path d="M6 -36 C14 -38 16 -30 10 -28" fill="#4e8a3e" />
      <path d="M-10 -6 L-2 6 L-12 8 Z" fill="#3a1408" />
      <path d="M8 -6 L16 8 L6 6 Z" fill="#3a1408" />
      <path d="M-8 12 Q0 22 10 12" fill="none" stroke="#3a1408" strokeWidth="1.6" />
    </g>
  )
}

function HauntedHouse() {
  return (
    <svg className="holiday-figure holiday-figure--tall" viewBox="0 0 220 190" aria-hidden>
      <defs>
        <linearGradient id="hw-wall" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="#3a2458" />
          <stop offset="100%" stopColor="#1a1024" />
        </linearGradient>
        <radialGradient id="hw-win" cx="50%" cy="40%" r="70%">
          <stop offset="0%" stopColor="#ffe7a0" />
          <stop offset="100%" stopColor="#e07a18" />
        </radialGradient>
      </defs>
      <path d="M18 188 H210" stroke="#120810" strokeWidth="8" />
      <path d="M36 112 H78 V188 H36 Z" fill="url(#hw-wall)" />
      <path d="M28 112 L57 62 L86 112 Z" fill="#241430" />
      <rect x="48" y="78" width="16" height="22" fill="#120810" />
      <path d="M70 188 V150 H108 V112 H168 V188 Z" fill="url(#hw-wall)" />
      <path d="M96 112 L138 58 L186 112 Z" fill="#1c1028" />
      <path d="M138 58 L146 48 V70" fill="none" stroke="#2a1840" strokeWidth="6" />
      <g className="holiday-smoke">
        <circle cx="148" cy="40" r="5" fill="#d8c8dc" opacity="0.35" />
        <circle cx="156" cy="30" r="7" fill="#efe6f2" opacity="0.28" />
        <circle cx="150" cy="18" r="4" fill="#fff" opacity="0.2" />
      </g>
      <Window x={118} y={128} />
      <Window x={148} y={128} />
      <Window x={132} y={86} />
      <path d="M52 148 H66 V168 H52 Z" fill="url(#hw-win)" />
      <path d="M59 148 V168 M52 158 H66" stroke="#2a1408" strokeWidth="1.2" />
      <path d="M124 160 H146 V188 H124 Z" fill="#120810" />
      <circle cx="142" cy="174" r="1.4" fill="#e8c56b" />
      <path d="M108 150 C118 140 128 148 136 136" fill="none" stroke="#243018" strokeWidth="2" />
      <path d="M8 188 L8 150 L16 142 L24 150 V188 M40 188 V158" fill="#120810" />
      <path d="M176 188 V162 H184 V188 M196 188 V150 L204 142 L212 150 V188" fill="#120810" />
    </svg>
  )
}

function Window({ x, y }: { x: number; y: number }) {
  return (
    <g>
      <rect className="holiday-window" x={x} y={y} width="18" height="22" rx="1" fill="url(#hw-win)" />
      <path d={`M${x + 9} ${y} V${y + 22} M${x} ${y + 11} H${x + 18}`} stroke="#2a1408" strokeWidth="1.3" />
    </g>
  )
}

function OrnateLantern() {
  return (
    <svg className="holiday-figure holiday-figure--tall" viewBox="0 0 90 200" aria-hidden>
      <defs>
        <radialGradient id="hw-flame" cx="50%" cy="35%" r="65%">
          <stop offset="0%" stopColor="#fff6d0" />
          <stop offset="40%" stopColor="#ffb03a" />
          <stop offset="100%" stopColor="#e05a10" />
        </radialGradient>
      </defs>
      <path d="M45 18 V46" stroke="#d7c4ae" strokeWidth="2" strokeLinecap="round" />
      <path d="M28 46 H62 L58 54 H32 Z" fill="#5a4030" />
      <path d="M26 54 H64 L58 118 H32 Z" fill="#2a1c18" />
      <path d="M31 60 H59 L54 112 H36 Z" fill="url(#hw-flame)" />
      <path d="M45 60 V112 M31 78 H59 M33 96 H57" stroke="#6a3014" strokeWidth="1.4" opacity="0.7" />
      <path d="M38 74 L45 90 L38 90 Z M48 74 L55 90 L48 90 Z" fill="#4a1c08" />
      <path d="M32 118 H58 L45 132 Z" fill="#e08a28" />
      <rect x="42" y="132" width="6" height="52" rx="2" fill="#3a2a22" />
      <path d="M24 184 H66 L62 198 H28 Z" fill="#241810" />
      <ellipse cx="45" cy="86" rx="8" ry="16" fill="#fff6d0" opacity="0.35" />
    </svg>
  )
}

export function NewYearFrieze() {
  return (
    <div className="holiday-frieze">
      <PineCluster />
      <Snowman />
      <Cottage />
      <GiftStack />
      <LampPost />
    </div>
  )
}

function PineCluster() {
  return (
    <svg className="holiday-figure holiday-figure--tall" viewBox="0 0 200 180" aria-hidden>
      <Pine x={46} y={168} h={120} snow />
      <Pine x={108} y={172} h={150} lights />
      <Pine x={162} y={168} h={108} snow />
    </svg>
  )
}

function Pine({ x, y, h, lights, snow }: { x: number; y: number; h: number; lights?: boolean; snow?: boolean }) {
  const tier = h / 3.2
  return (
    <g transform={`translate(${x} ${y})`}>
      <rect x={-4} y={-16} width={8} height={18} fill="#5c4030" />
      {[0, 1, 2].map((i) => {
        const top = -18 - i * tier * 0.72
        const half = 18 + (2 - i) * 14
        return (
          <g key={i}>
            <path d={`M0 ${top - tier} L${half} ${top} L${-half} ${top} Z`} fill={i === 2 ? '#1e6b4a' : '#176346'} />
            {snow ? <path d={`M0 ${top - tier + 6} L${half * 0.45} ${top - 4} L${-half * 0.42} ${top - 4} Z`} fill="#f4fbff" opacity="0.92" /> : null}
          </g>
        )
      })}
      {lights
        ? [
            [-10, -40, '#e23b4a'],
            [12, -58, '#f2d36b'],
            [-6, -78, '#7ee0ff'],
            [8, -96, '#c084fc'],
            [0, -28, '#3ecf8e'],
          ].map(([lx, ly, fill], i) => <circle key={i} className="holiday-bulb" cx={Number(lx)} cy={Number(ly)} r="3.2" fill={String(fill)} />)
        : null}
    </g>
  )
}

function Snowman() {
  return (
    <svg className="holiday-figure holiday-figure--tall" viewBox="0 0 160 200" aria-hidden>
      <defs>
        <radialGradient id="ny-snow" cx="35%" cy="32%" r="68%">
          <stop offset="0%" stopColor="#ffffff" />
          <stop offset="72%" stopColor="#e7f3fb" />
          <stop offset="100%" stopColor="#c5d8e6" />
        </radialGradient>
      </defs>
      <ellipse cx="80" cy="190" rx="46" ry="7" fill="#9bb8cc" opacity="0.4" />
      <circle cx="80" cy="158" r="34" fill="url(#ny-snow)" />
      <circle cx="80" cy="108" r="25" fill="url(#ny-snow)" />
      <circle cx="80" cy="70" r="18" fill="url(#ny-snow)" />
      <path d="M55 104 L18 84" stroke="#6b4428" strokeWidth="3.2" strokeLinecap="round" />
      <path d="M18 84 L8 76 M18 84 L12 94" stroke="#6b4428" strokeWidth="2.2" strokeLinecap="round" />
      <path d="M105 104 L142 84" stroke="#6b4428" strokeWidth="3.2" strokeLinecap="round" />
      <path d="M142 84 L152 76 M142 84 L148 94" stroke="#6b4428" strokeWidth="2.2" strokeLinecap="round" />
      <path d="M62 84 H98 Q100 96 80 98 Q60 96 62 84Z" fill="#d23b4a" />
      <path d="M92 90 L100 124 H88 L86 92Z" fill="#d23b4a" />
      <path d="M90 124 H100 M92 128 H98" stroke="#f2d36b" strokeWidth="1.6" strokeLinecap="round" />
      <circle cx="73" cy="68" r="2.1" fill="#243044" />
      <circle cx="87" cy="68" r="2.1" fill="#243044" />
      <path d="M84 72 L98 76 L84 78 Z" fill="#e07a22" />
      <path d="M72 78 Q80 84 88 78" fill="none" stroke="#243044" strokeWidth="1.3" strokeLinecap="round" />
      <circle cx="66" cy="74" r="2.3" fill="#f0a0b0" opacity="0.65" />
      <circle cx="94" cy="74" r="2.3" fill="#f0a0b0" opacity="0.65" />
      <circle cx="80" cy="118" r="2.3" fill="#243044" />
      <circle cx="80" cy="130" r="2.3" fill="#243044" />
      <circle cx="80" cy="156" r="2.5" fill="#243044" />
      <circle cx="80" cy="170" r="2.5" fill="#243044" />
      <rect x="66" y="28" width="28" height="22" rx="2" fill="#1c2430" />
      <rect x="66" y="42" width="28" height="5" fill="#d23b4a" />
      <ellipse cx="80" cy="52" rx="22" ry="5" fill="#1c2430" />
    </svg>
  )
}

function Cottage() {
  return (
    <svg className="holiday-figure holiday-figure--tall" viewBox="0 0 230 170" aria-hidden>
      <defs>
        <linearGradient id="ny-wall" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="#d7e7f2" />
          <stop offset="100%" stopColor="#8eafc4" />
        </linearGradient>
        <radialGradient id="ny-win" cx="50%" cy="40%" r="70%">
          <stop offset="0%" stopColor="#fff1c4" />
          <stop offset="100%" stopColor="#e08a22" />
        </radialGradient>
      </defs>
      <path d="M18 158 H214" stroke="#f7fbff" strokeWidth="10" strokeLinecap="round" />
      <path d="M48 96 H168 V150 H48 Z" fill="url(#ny-wall)" />
      <path d="M36 100 L108 42 L180 100 Z" fill="#f4fbff" />
      <path d="M48 96 L108 52 L168 96" fill="none" stroke="#d5e8f4" strokeWidth="8" />
      <path d="M146 78 H162 V108 H146 Z" fill="#6e5848" />
      <path d="M140 78 H168 L164 70 H144 Z" fill="#f7fbff" />
      <g className="holiday-smoke">
        <circle cx="154" cy="62" r="5" fill="#fff" opacity="0.55" />
        <circle cx="162" cy="50" r="7" fill="#fff" opacity="0.4" />
        <circle cx="156" cy="36" r="4" fill="#fff" opacity="0.3" />
      </g>
      <rect x="64" y="112" width="22" height="20" rx="1" fill="url(#ny-win)" />
      <rect x="98" y="112" width="22" height="20" rx="1" fill="url(#ny-win)" />
      <path d="M75 112 V132 M64 122 H86 M109 112 V132 M98 122 H120" stroke="#6a4020" strokeWidth="1.3" />
      <path d="M128 118 H150 V150 H128 Z" fill="#6e5848" />
      <circle cx="146" cy="134" r="1.5" fill="#e8c56b" />
      <circle cx="139" cy="108" r="9" fill="none" stroke="#2f8a4e" strokeWidth="2.4" />
      <path d="M139 100 L141 108 L139 116 M131 108 H147" stroke="#d23b4a" strokeWidth="1.6" />
      {Array.from({ length: 9 }, (_, i) => (
        <path key={i} d={`M${52 + i * 13} 98 L${56 + i * 13} ${110 + (i % 3) * 4} L${60 + i * 13} 98`} fill="#f7fbff" opacity="0.9" />
      ))}
    </svg>
  )
}

function GiftStack() {
  return (
    <svg className="holiday-figure holiday-figure--short" viewBox="0 0 140 150" aria-hidden>
      <rect x="12" y="92" width="116" height="50" rx="4" fill="#d23b4a" />
      <rect x="64" y="92" width="12" height="50" fill="#f2d36b" />
      <rect x="12" y="110" width="116" height="10" fill="#f2d36b" />
      <rect x="28" y="54" width="84" height="38" rx="3" fill="#1f8a62" />
      <rect x="64" y="54" width="12" height="38" fill="#f7fbff" />
      <rect x="28" y="68" width="84" height="9" fill="#f7fbff" />
      <rect x="44" y="24" width="52" height="30" rx="3" fill="#3d7ec4" />
      <rect x="66" y="24" width="8" height="30" fill="#f2d36b" />
      <rect x="44" y="35" width="52" height="7" fill="#f2d36b" />
      <ellipse cx="62" cy="22" rx="9" ry="6" fill="#f2d36b" />
      <ellipse cx="78" cy="22" rx="9" ry="6" fill="#e8c56b" />
      <circle cx="70" cy="23" r="3.4" fill="#fff4c4" />
    </svg>
  )
}

function LampPost() {
  return (
    <svg className="holiday-figure holiday-figure--tall" viewBox="0 0 80 180" aria-hidden>
      <defs>
        <radialGradient id="ny-lamp" cx="50%" cy="40%" r="60%">
          <stop offset="0%" stopColor="#fff6d0" />
          <stop offset="100%" stopColor="#e0a040" />
        </radialGradient>
      </defs>
      <path d="M40 176 V78" stroke="#243044" strokeWidth="5" />
      <path d="M28 176 H52" stroke="#243044" strokeWidth="4" strokeLinecap="round" />
      <path d="M24 70 H56 L52 96 H28 Z" fill="#1c2836" />
      <path d="M28 74 H52 L49 92 H31 Z" fill="url(#ny-lamp)" />
      <path d="M40 70 V58" stroke="#243044" strokeWidth="3" />
      <path d="M26 58 H54" stroke="#f7fbff" strokeWidth="6" strokeLinecap="round" />
      <circle cx="40" cy="104" r="8" fill="none" stroke="#2f8a4e" strokeWidth="2.2" />
      <path d="M40 97 V111 M34 104 H46" stroke="#d23b4a" strokeWidth="1.5" />
    </svg>
  )
}
