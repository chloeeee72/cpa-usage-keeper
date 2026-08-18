import { useEffect, useMemo, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Select, type SelectOption } from '@/components/ui/Select'
import styles from './PeakTimeEditor.module.scss'

export interface PeakTimeRangeDraft {
  start: string
  end: string
}

export interface PeakTimeEditorProps {
  value?: string | null
  onChange: (value: string | null) => void
  disabled?: boolean
}

const TIME_OPTIONS: SelectOption[] = Array.from({ length: 96 }, (_, index) => {
  const hour = Math.floor(index / 4)
  const minute = (index % 4) * 15
  const label = `${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}`
  return { value: label, label }
})

const DEFAULT_RANGES: PeakTimeRangeDraft[] = [
  { start: '09:00', end: '12:00' },
  { start: '14:00', end: '18:00' },
]

const parseConfig = (value?: string | null): { timezone: string; ranges: PeakTimeRangeDraft[] } => {
  if (!value) return { timezone: 'Asia/Shanghai', ranges: DEFAULT_RANGES }
  try {
    const parsed = JSON.parse(value) as { timezone?: string; ranges?: PeakTimeRangeDraft[] }
    return {
      timezone: parsed.timezone || 'Asia/Shanghai',
      ranges: Array.isArray(parsed.ranges) && parsed.ranges.length > 0 ? parsed.ranges : DEFAULT_RANGES,
    }
  } catch {
    return { timezone: 'Asia/Shanghai', ranges: DEFAULT_RANGES }
  }
}

export function PeakTimeEditor({ value, onChange, disabled }: PeakTimeEditorProps) {
  const [timezone, setTimezone] = useState('Asia/Shanghai')
  const [ranges, setRanges] = useState<PeakTimeRangeDraft[]>(DEFAULT_RANGES)

  useEffect(() => {
    const parsed = parseConfig(value)
    setTimezone(parsed.timezone)
    setRanges(parsed.ranges)
  }, [value])

  const emit = (nextTimezone: string, nextRanges: PeakTimeRangeDraft[]) => {
    const validRanges = nextRanges.filter((range) => range.start && range.end)
    if (validRanges.length === 0) {
      onChange(null)
      return
    }
    onChange(JSON.stringify({ timezone: nextTimezone || 'Asia/Shanghai', ranges: validRanges }))
  }

  const updateRange = (index: number, patch: Partial<PeakTimeRangeDraft>) => {
    const next = ranges.map((range, i) => (i === index ? { ...range, ...patch } : range))
    setRanges(next)
    emit(timezone, next)
  }

  const addRange = () => {
    const next = [...ranges, { start: '00:00', end: '01:00' }]
    setRanges(next)
    emit(timezone, next)
  }

  const removeRange = (index: number) => {
    const next = ranges.filter((_, i) => i !== index)
    setRanges(next)
    emit(timezone, next)
  }

  const timeOptions = useMemo(() => TIME_OPTIONS, [])

  return (
    <div className={styles.container}>
      <div className={styles.row}>
        <label>Timezone</label>
        <Input
          value={timezone}
          onChange={(event) => {
            setTimezone(event.target.value)
            emit(event.target.value, ranges)
          }}
          disabled={disabled}
        />
      </div>
      {ranges.map((range, index) => (
        <div key={index} className={styles.row}>
          <Select
            value={range.start}
            options={timeOptions}
            onChange={(value) => updateRange(index, { start: value })}
            disabled={disabled}
          />
          <span>→</span>
          <Select
            value={range.end}
            options={timeOptions}
            onChange={(value) => updateRange(index, { end: value })}
            disabled={disabled}
          />
          <Button variant="danger" size="sm" appearance="action" onClick={() => removeRange(index)} disabled={disabled || ranges.length <= 1}>
            Remove
          </Button>
        </div>
      ))}
      <Button variant="secondary" size="sm" appearance="action" onClick={addRange} disabled={disabled}>
        Add peak range
      </Button>
    </div>
  )
}
