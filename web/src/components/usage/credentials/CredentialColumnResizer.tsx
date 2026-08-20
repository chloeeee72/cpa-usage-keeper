import { useCallback, useRef, type CSSProperties, type PointerEvent } from 'react'
import styles from './CredentialSections.module.scss'

interface CredentialColumnResizerProps {
  onResize: (deltaX: number) => void
  ariaLabel: string
  style: CSSProperties
}

export function CredentialColumnResizer({ onResize, ariaLabel, style }: CredentialColumnResizerProps) {
  const startXRef = useRef<number | null>(null)

  const handlePointerDown = useCallback((event: PointerEvent<HTMLDivElement>) => {
    event.preventDefault()
    startXRef.current = event.clientX
    event.currentTarget.setPointerCapture(event.pointerId)
  }, [])

  const handlePointerMove = useCallback((event: PointerEvent<HTMLDivElement>) => {
    if (startXRef.current === null) return
    const deltaX = event.clientX - startXRef.current
    if (deltaX === 0) return
    startXRef.current = event.clientX
    onResize(deltaX)
  }, [onResize])

  const handlePointerUp = useCallback((event: PointerEvent<HTMLDivElement>) => {
    startXRef.current = null
    try {
      event.currentTarget.releasePointerCapture(event.pointerId)
    } catch {
      // 指针可能已经释放，忽略。
    }
  }, [])

  const handlePointerCancel = useCallback(() => {
    startXRef.current = null
  }, [])

  return (
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label={ariaLabel}
      className={styles.credentialColumnResizer}
      style={style}
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerUp}
      onPointerCancel={handlePointerCancel}
    />
  )
}
