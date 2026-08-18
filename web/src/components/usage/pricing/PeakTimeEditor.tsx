import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import styles from './PeakTimeEditor.module.scss'

export interface PeakTimeRangeDraft {
  start: string
  end: string
}

export interface PeakTimeEditorProps {
  value?: string | null
  onChange: (value: string | null) => void
  disabled?: boolean
  /** When provided, the timezone is rendered outside this editor and controlled by the parent. */
  timezone?: string
  onTimezoneChange?: (timezone: string) => void
  /** Maximum number of peak ranges that can be displayed. Defaults to 3 (1 initial + 2 additions). */
  maxRanges?: number
}

const DEFAULT_TIMEZONE = 'Asia/Shanghai'

const DEFAULT_RANGES: PeakTimeRangeDraft[] = [
  { start: '09:00', end: '12:00' },
]

const DEFAULT_MAX_RANGES = 3

export const parsePeakTimeConfig = (value?: string | null): { timezone: string; ranges: PeakTimeRangeDraft[] } => {
  if (!value) return { timezone: DEFAULT_TIMEZONE, ranges: DEFAULT_RANGES }
  try {
    const parsed = JSON.parse(value) as { timezone?: string; ranges?: PeakTimeRangeDraft[] }
    return {
      timezone: parsed.timezone || DEFAULT_TIMEZONE,
      ranges: Array.isArray(parsed.ranges) && parsed.ranges.length > 0 ? parsed.ranges : DEFAULT_RANGES,
    }
  } catch {
    return { timezone: DEFAULT_TIMEZONE, ranges: DEFAULT_RANGES }
  }
}

export function PeakTimeEditor({ value, onChange, disabled, timezone, onTimezoneChange, maxRanges = DEFAULT_MAX_RANGES }: PeakTimeEditorProps) {
  const { t } = useTranslation()
  const parsed = parsePeakTimeConfig(value)
  const resolvedTimezone = timezone ?? parsed.timezone
  const ranges = parsed.ranges

  const emit = (nextTimezone: string, nextRanges: PeakTimeRangeDraft[]) => {
    const validRanges = nextRanges.filter((range) => range.start && range.end)
    if (validRanges.length === 0) {
      onChange(null)
      return
    }
    onChange(JSON.stringify({ timezone: nextTimezone || DEFAULT_TIMEZONE, ranges: validRanges }))
  }

  const updateTimezone = (nextTimezone: string) => {
    onTimezoneChange?.(nextTimezone)
    emit(nextTimezone, ranges)
  }

  const updateRange = (index: number, patch: Partial<PeakTimeRangeDraft>) => {
    const next = ranges.map((range, i) => (i === index ? { ...range, ...patch } : range))
    emit(resolvedTimezone, next)
  }

  const addRange = () => {
    if (ranges.length >= maxRanges) return
    const next = [...ranges, { start: '00:00', end: '01:00' }]
    emit(resolvedTimezone, next)
  }

  const removeRange = (index: number) => {
    const next = ranges.filter((_, i) => i !== index)
    emit(resolvedTimezone, next)
  }

  const showInlineTimezone = !onTimezoneChange

  return (
    <div className={styles.container}>
      {showInlineTimezone && (
        <div className={styles.row}>
          <label>{t('usage_stats.model_price_peak_time_timezone')}</label>
          <Input
            value={resolvedTimezone}
            onChange={(event) => updateTimezone(event.target.value)}
            disabled={disabled}
          />
          <Button
            variant="secondary"
            size="sm"
            appearance="action"
            className={styles.addButton}
            onClick={addRange}
            disabled={disabled || ranges.length >= maxRanges}
          >
            {t('usage_stats.model_price_peak_time_add')}
          </Button>
        </div>
      )}

      <div className={styles.ranges}>
        {!showInlineTimezone && (
          <Button
            variant="secondary"
            size="sm"
            appearance="action"
            className={styles.addButton}
            onClick={addRange}
            disabled={disabled || ranges.length >= maxRanges}
          >
            {t('usage_stats.model_price_peak_time_add')}
          </Button>
        )}

        {ranges.map((range, index) => (
          <div key={index} className={styles.range}>
            <div className={styles.rangeHeader}>
              <span className={styles.rangeLabel}>
                {t('usage_stats.model_price_peak_range')} {index + 1}
              </span>
              {ranges.length > 1 && (
                <Button
                  variant="danger"
                  size="sm"
                  appearance="action"
                  onClick={() => removeRange(index)}
                  disabled={disabled}
                >
                  {t('usage_stats.model_price_peak_time_remove')}
                </Button>
              )}
            </div>
            <div className={styles.rangeControl}>
              <Input
                type="time"
                step="60"
                value={range.start}
                onChange={(event) => updateRange(index, { start: event.target.value })}
                placeholder="Start time"
                aria-label={`${t('usage_stats.model_price_peak_range')} ${index + 1} start`}
                disabled={disabled}
              />
              <span className={styles.separator}>To</span>
              <Input
                type="time"
                step="60"
                value={range.end}
                onChange={(event) => updateRange(index, { end: event.target.value })}
                placeholder="End time"
                aria-label={`${t('usage_stats.model_price_peak_range')} ${index + 1} end`}
                disabled={disabled}
              />
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
