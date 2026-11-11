import { AnimatePresence, motion, useReducedMotion, type Transition } from 'framer-motion'
import { useEffect, useMemo, useRef } from 'react'

export interface ProgressiveFluxPhase { at: number; label: string }

interface ProgressiveFluxLoaderProps {
  value?: number
  phases?: ProgressiveFluxPhase[]
  showLabel?: boolean
  onComplete?: () => void
  className?: string
}

const DEFAULT_PHASES: ProgressiveFluxPhase[] = [
  { at: 0, label: 'starting up' },
  { at: 25, label: 'loading assets' },
  { at: 55, label: 'preparing magic' },
  { at: 80, label: 'almost there' },
  { at: 100, label: 'all done' },
]

const Z_TRANSITION: Transition = { duration: 0.9, ease: [0.22, 1, 0.36, 1] }
const LETTER_TRANSITION: Transition = { duration: 0.45, ease: [0.22, 1, 0.36, 1] }
const FLUX_FROM = 'var(--flux-from, #1d6ffb)'
const FLUX_TO = 'var(--flux-to, #74e1ff)'
const FLUX_MID = `color-mix(in oklab, ${FLUX_FROM}, ${FLUX_TO})`
const DEFAULT_GRADIENT = `linear-gradient(90deg, ${FLUX_FROM} 0%, ${FLUX_MID} 35%, ${FLUX_TO} 55%, ${FLUX_MID} 78%, ${FLUX_FROM} 100%)`

function pickLabel(value: number, phases: ProgressiveFluxPhase[]) {
  let active = phases[0]?.label ?? ''
  for (const phase of phases) if (value >= phase.at) active = phase.label
  return active
}

function FluxLabel({ label, reduced }: { label: string; reduced: boolean }) {
  const className = 'flux-label'
  if (reduced) return <div className={className} aria-hidden>{label}</div>
  return <AnimatePresence mode="wait"><motion.div key={label} className={className} aria-hidden style={{ transformStyle: 'preserve-3d', originX: 0, originY: 0.5 }} initial={{ opacity: 0, z: -380, scale: 0.65, filter: 'blur(14px)' }} animate={{ opacity: [0, 1, 1, 1], z: [-380, 60, -8, 0], scale: [0.65, 1.08, 0.985, 1], filter: ['blur(14px)', 'blur(0px)', 'blur(0px)', 'blur(0px)'] }} exit={{ opacity: 0, z: 220, scale: 1.2, filter: 'blur(10px)', transition: { duration: 0.45, ease: [0.7, 0, 0.84, 0] } }} transition={Z_TRANSITION}><span>{label.split('').map((char, index) => <motion.span key={`${label}-${index}`} initial={{ opacity: 0, y: 12, filter: 'blur(8px)' }} animate={{ opacity: 1, y: 0, filter: 'blur(0px)' }} transition={{ ...LETTER_TRANSITION, delay: 0.18 + index * 0.035 }}>{char === ' ' ? '\u00a0' : char}</motion.span>)}</span></motion.div></AnimatePresence>
}

export function ProgressiveFluxLoader({ value = 0, phases = DEFAULT_PHASES, showLabel = true, onComplete, className = '' }: ProgressiveFluxLoaderProps) {
  const reduced = Boolean(useReducedMotion())
  const current = Number.isFinite(value) ? Math.min(100, Math.max(0, value)) : 0
  const rounded = Math.round(current)
  const sortedPhases = useMemo(() => [...phases].sort((a, b) => a.at - b.at), [phases])
  const label = useMemo(() => pickLabel(current, sortedPhases), [current, sortedPhases])
  const completed = useRef(false)

  useEffect(() => {
    if (current >= 100 && !completed.current) { completed.current = true; onComplete?.() }
    if (current < 100) completed.current = false
  }, [current, onComplete])

  return <div className={`progressive-flux-loader ${className}`}><div className="flux-label-wrap">{showLabel && <FluxLabel label={label} reduced={reduced} />}</div><div className="flux-track" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={rounded} aria-valuetext={`${rounded}% – ${label}`} aria-label="Indexing progress"><motion.div className="flux-fill" style={{ background: DEFAULT_GRADIENT }} initial={false} animate={{ width: `${current}%` }} transition={reduced ? { duration: 0 } : { duration: 0.55, ease: [0.22, 1, 0.36, 1] }}>{!reduced && <motion.span className="flux-sheen" aria-hidden animate={{ x: ['-110%', '210%'] }} transition={{ duration: 1.6, ease: 'linear', repeat: Infinity }} />}</motion.div></div></div>
}

export default ProgressiveFluxLoader
