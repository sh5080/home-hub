// 콩이: 받침대 위의 강낭콩 캐릭터. 그라데이션 없이 단색 + 아래 두께(압출) + 안쪽 테두리 선으로 입체를 낸다.
// 팔레트: 네이비 바탕 위 금빛 콩, 청록 잎, 크림 꽃. 단계마다 머리 위 새싹이 자란다.
const C = {
  soilTop: '#9a6a44', soilFace: '#76502f', soilDeep: '#4c321d', soilRim: '#b8875c', pebble: '#c9a07a', pocket: '#5e3e23',
  bean: '#e2c15f', beanSide: '#a8862f', beanRim: '#f3dc8e',
  leaf: '#6fae8f', leafSide: '#3f7564', leafRim: '#9fd1b5',
  cream: '#f3ebdd', creamSide: '#c9bda6', gold: '#e2b84a',
  ink: '#22334f', blush: '#e89a7c',
}
// 흙: 둥근 윗면 + 아래로 두툼한 단면
const SOIL_TOP = 'M20 142 C20 128 52 124 100 124 C148 124 180 128 180 142 C180 150 148 154 100 154 C52 154 20 150 20 142Z'
const SOIL_FACE = 'M20 142 C20 150 52 154 100 154 C148 154 180 150 180 142 L180 176 C180 188 150 194 100 194 C50 194 20 188 20 176Z'
const BEAN = 'M0 -24 C6 -28 15 -29 22 -26 C31 -22 36 -12 36 2 C36 18 22 28 0 28 C-22 28 -36 18 -36 2 C-36 -12 -31 -22 -22 -26 C-15 -29 -6 -28 0 -24Z'

export function BeanBuddy({ stage, size = 160, happy = false }: { stage: number; size?: number; happy?: boolean }) {
  const smile = happy || stage === 5
  const sleeping = stage === 1
  const bx = 100, by = stage === 1 ? 166 : stage === 2 ? 115 : 106 // 콩 중심(1 흙 속, 2 반쯤, 3~ 흙 위)

  // 납작한 압출 도형: 아래로 d 만큼 두께, 위 면, 안쪽 테두리
  const Extrude = ({ d, top, side, rim, depth = 5, rimScale = 0.86 }: { d: string; top: string; side: string; rim?: string; depth?: number; rimScale?: number }) => (
    <g>
      <path d={d} fill={side} transform={`translate(0 ${depth})`} />
      <path d={d} fill={top} />
      {rim && <path d={d} fill="none" stroke={rim} strokeWidth={2.2 / rimScale} transform={`scale(${rimScale})`} opacity="0.9" />}
    </g>
  )
  const leafD = (len: number) => `M0 0 C${len * 0.55} ${-len * 0.12} ${len * 0.5} ${-len * 0.85} 0 ${-len} C${-len * 0.5} ${-len * 0.85} ${-len * 0.55} ${-len * 0.12} 0 0Z`
  const Leaf = ({ x, y, len, rot }: { x: number; y: number; len: number; rot: number }) => (
    <g transform={`translate(${x} ${y}) rotate(${rot})`}>
      <path d={leafD(len)} fill={C.leafSide} transform="translate(1.6 2.4)" />
      <path d={leafD(len)} fill={C.leaf} />
      <path d={`M0 ${-len * 0.15} L0 ${-len * 0.78}`} stroke={C.leafRim} strokeWidth="1.6" strokeLinecap="round" />
    </g>
  )
  const Star = ({ x, y, r, o = 1 }: { x: number; y: number; r: number; o?: number }) => (
    <path d={`M${x} ${y - r} L${x + r * 0.28} ${y - r * 0.28} L${x + r} ${y} L${x + r * 0.28} ${y + r * 0.28} L${x} ${y + r} L${x - r * 0.28} ${y + r * 0.28} L${x - r} ${y} L${x - r * 0.28} ${y - r * 0.28}Z`} fill={C.gold} opacity={o} />
  )

  // 머리 위 새싹
  const top = by - 26
  const stemH = [0, 0, 12, 18, 26, 30][stage]
  const tip = top - stemH
  const sprout = stage >= 2 && (
    <g className="plant-sway" style={{ transformOrigin: `${bx}px ${top}px` }}>
      <path d={`M${bx} ${top} C${bx + 2} ${top - stemH * 0.4} ${bx - 2} ${top - stemH * 0.7} ${bx} ${tip}`} stroke={C.leafSide} strokeWidth="5" fill="none" strokeLinecap="round" transform="translate(1.2 1.6)" />
      <path d={`M${bx} ${top} C${bx + 2} ${top - stemH * 0.4} ${bx - 2} ${top - stemH * 0.7} ${bx} ${tip}`} stroke={C.leaf} strokeWidth="4.4" fill="none" strokeLinecap="round" />
      {stage >= 4 && <Leaf x={bx} y={top - stemH * 0.4} len={15} rot={-70} />}
      {stage >= 4 && <Leaf x={bx} y={top - stemH * 0.4} len={15} rot={70} />}
      <Leaf x={bx} y={tip + 2} len={[0, 0, 13, 18, 20, 18][stage]} rot={-48} />
      <Leaf x={bx} y={tip + 2} len={[0, 0, 13, 18, 20, 18][stage]} rot={48} />
      {stage === 4 && (
        <g transform={`translate(${bx} ${tip - 6})`}>
          <ellipse cx="1" cy="2.6" rx="7" ry="9" fill={C.creamSide} />
          <ellipse cx="0" cy="0" rx="7" ry="9" fill={C.cream} />
          <path d="M-7 4 q7 6 14 0 l-1.5 -4 q-5.5 3 -11 0z" fill={C.leaf} />
        </g>
      )}
      {stage === 5 && (
        <g transform={`translate(${bx} ${tip - 9})`}>
          {[0, 72, 144, 216, 288].map((a) => (
            <g key={a} transform={`rotate(${a})`}>
              <ellipse cx="0.8" cy="-8.4" rx="6.6" ry="8.6" fill={C.creamSide} />
              <ellipse cx="0" cy="-10" rx="6.6" ry="8.6" fill={C.cream} />
            </g>
          ))}
          <circle cx="0.6" cy="1.6" r="6" fill="#b88a26" />
          <circle cx="0" cy="0" r="6" fill={C.gold} />
          <circle cx="-1.8" cy="-1.8" r="1.6" fill={C.cream} opacity="0.85" />
        </g>
      )}
    </g>
  )

  const face = (
    <g>
      {sleeping ? (
        <g stroke={C.ink} strokeWidth="2.4" fill="none" strokeLinecap="round">
          <path d="M-14 2 q4 3.5 8 0" /><path d="M6 2 q4 3.5 8 0" />
        </g>
      ) : smile ? (
        <g stroke={C.ink} strokeWidth="2.6" fill="none" strokeLinecap="round">
          <path d="M-14 3 q4 -5 8 0" /><path d="M6 3 q4 -5 8 0" />
        </g>
      ) : (
        <g className="plant-blink">
          <ellipse cx="-10" cy="2" rx="3.4" ry="4.4" fill={C.ink} />
          <ellipse cx="10" cy="2" rx="3.4" ry="4.4" fill={C.ink} />
          <circle cx="-9" cy="0.2" r="1.3" fill={C.cream} />
          <circle cx="11" cy="0.2" r="1.3" fill={C.cream} />
        </g>
      )}
      <ellipse cx="-20" cy="10" rx="5" ry="3" fill={C.blush} opacity="0.75" />
      <ellipse cx="20" cy="10" rx="5" ry="3" fill={C.blush} opacity="0.75" />
      {sleeping ? <ellipse cx="0" cy="11" rx="2.2" ry="1.6" fill={C.ink} /> : <path d={smile ? 'M-5 9 q5 6 10 0z' : 'M-3.4 9.4 q3.4 3 6.8 0'} stroke={C.ink} strokeWidth="2" fill={smile ? C.ink : 'none'} strokeLinecap="round" strokeLinejoin="round" />}
    </g>
  )

  return (
    <svg viewBox="0 0 200 200" width={size} height={size} role="img" aria-label={`콩이 ${stage}단계`} className="overflow-visible">
      <defs>
        <radialGradient id="bb-shadow" cx="0.5" cy="0.5" r="0.5"><stop offset="0" stopColor="#0b1424" stopOpacity="0.55" /><stop offset="1" stopColor="#0b1424" stopOpacity="0" /></radialGradient>
      </defs>
      {/* 반짝이(배경) */}
      <Star x={32} y={34} r={3.2} o={0.7} />
      <Star x={170} y={22} r={2.4} o={0.55} />
      <Star x={178} y={88} r={2} o={0.45} />
      {stage === 5 && <><Star x={48} y={78} r={4} /><Star x={156} y={60} r={4.6} /></>}

      {/* 흙: 그림자 → 단면(앞면) → 윗면 → 테두리선 → 돌멩이 */}
      <ellipse cx="100" cy="192" rx="90" ry="9" fill="url(#bb-shadow)" />
      <path d={SOIL_FACE} fill={C.soilFace} />
      <path d={SOIL_FACE} fill="none" stroke={C.soilDeep} strokeWidth="2" opacity="0.5" />
      {[[48, 160, 3.2], [150, 166, 2.6], [70, 178, 2.2], [132, 182, 3], [40, 178, 1.8], [160, 152, 2]].map(([x, y, r], i) => (
        <ellipse key={i} cx={x} cy={y} rx={r} ry={r * 0.7} fill={C.pebble} opacity="0.8" />
      ))}
      <path d={SOIL_TOP} fill={C.soilTop} />
      <path d="M30 138 C60 132 140 132 170 138" stroke={C.soilRim} strokeWidth="2" fill="none" strokeLinecap="round" opacity="0.8" />
      {/* 흙 위 작은 풀 */}
      {[[36, 136], [58, 132], [146, 132], [164, 136]].map(([x, y], i) => (
        <g key={i} fill={C.leaf}><path d={`M${x} ${y} q-2 -6 -1 -9 q2 4 2 9z`} /><path d={`M${x + 1} ${y} q2 -6 5 -7 q-2 4 -3 7z`} /></g>
      ))}
      {stage === 1 && <ellipse cx="100" cy="166" rx="40" ry="26" fill={C.pocket} opacity="0.6" />}
      {/* 캐릭터 크게(흙 윗면 기준) */}
      <g transform={stage === 1 ? 'translate(100 166) scale(0.86) translate(-100 -166)' : 'translate(100 140) scale(1.28) translate(-100 -140)'}>
      <g className={stage >= 2 ? 'plant-idle' : ''} style={{ transformOrigin: `${bx}px 150px` }}>
        <g>
          <g transform={`translate(${bx} ${by}) ${stage === 1 ? 'rotate(-6)' : ''}`}>
            {stage >= 3 && (
              <>
                <rect x="-20" y="22" width="14" height="10" rx="5" fill={C.beanSide} />
                <rect x="6" y="22" width="14" height="10" rx="5" fill={C.beanSide} />
              </>
            )}
            {stage >= 2 && <ellipse cx="-37" cy="8" rx="5" ry="7.4" fill={C.beanSide} transform="rotate(20 -37 8)" />}
            {stage >= 2 && <ellipse cx="37" cy="8" rx="5" ry="7.4" fill={C.beanSide} transform={smile ? 'rotate(-150 37 8)' : 'rotate(-20 37 8)'} />}
            <Extrude d={BEAN} top={C.bean} side={C.beanSide} rim={C.beanRim} depth={6} />
            {face}
          </g>
        </g>
        {sprout}
      </g>
      </g>
      {stage === 2 && (
        <g>
          <path d={SOIL_FACE} fill={C.soilFace} />
          <path d={SOIL_TOP} fill={C.soilTop} />
          <path d="M30 138 C60 132 140 132 170 138" stroke={C.soilRim} strokeWidth="2" fill="none" strokeLinecap="round" opacity="0.8" />
          <path d="M66 138 C74 128 86 126 92 134 M108 134 C114 126 126 128 134 138" stroke={C.soilRim} strokeWidth="3" fill="none" strokeLinecap="round" />
          {[[48, 160, 3.2], [150, 166, 2.6], [70, 178, 2.2], [132, 182, 3]].map(([x, y, r], i) => (
            <ellipse key={i} cx={x} cy={y} rx={r} ry={r * 0.7} fill={C.pebble} opacity="0.8" />
          ))}
        </g>
      )}
      {stage === 1 && (
        <g fill={C.cream} opacity="0.85" fontFamily="system-ui" fontWeight="800">
          <text x="140" y="150" fontSize="12">z</text><text x="150" y="138" fontSize="9.5">z</text><text x="158" y="128" fontSize="7.5">z</text>
        </g>
      )}
    </svg>
  )
}
