import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { fetchPeakHours, updatePeakHours, type PeakTimeRange } from '@/lib/api'
import styles from './PeakHoursModal.module.scss'

export interface PeakHoursModalProps {
  open: boolean
  onClose: () => void
  onNotice?: (kind: 'success' | 'info' | 'error', message: string) => void
}

const defaultRanges = (): PeakTimeRange[] => [
  { start: '09:00', end: '12:00' },
  { start: '14:00', end: '18:00' },
]

export function PeakHoursModal({ open, onClose, onNotice }: PeakHoursModalProps) {
  const { t } = useTranslation()
  const [timezone, setTimezone] = useState('Asia/Shanghai')
  const [ranges, setRanges] = useState<PeakTimeRange[]>(defaultRanges())
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!open) return
    let cancelled = false
    setLoading(true)
    fetchPeakHours()
      .then((config) => {
        if (cancelled) return
        setTimezone(config.timezone)
        setRanges(config.ranges.length > 0 ? config.ranges : defaultRanges())
      })
      .catch(() => {
        if (!cancelled) onNotice?.('error', 'Failed to load peak hours')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [open, onNotice])

  const updateRange = (index: number, patch: Partial<PeakTimeRange>) => {
    setRanges((current) => current.map((range, i) => (i === index ? { ...range, ...patch } : range)))
  }

  const save = async () => {
    if (saving) return
    setSaving(true)
    try {
      await updatePeakHours({ timezone: timezone.trim() || 'Asia/Shanghai', ranges })
      onNotice?.('success', 'Peak hours updated')
      onClose()
    } catch {
      onNotice?.('error', 'Failed to update peak hours')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal
      open={open}
      title="Peak hours"
      onClose={onClose}
      closeDisabled={saving}
      footer={
        <div className={styles.footer}>
          <Button variant="secondary" appearance="action" onClick={onClose} disabled={saving}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" appearance="action" onClick={() => void save()} loading={saving}>
            {t('common.save')}
          </Button>
        </div>
      }
      width={480}
    >
      {loading ? (
        <div className={styles.loading}>{t('common.loading')}</div>
      ) : (
        <div className={styles.body}>
          <div className={styles.field}>
            <label>Timezone</label>
            <Input
              value={timezone}
              onChange={(event) => setTimezone(event.target.value)}
              placeholder="Asia/Shanghai"
              disabled={saving}
            />
          </div>

          <div className={styles.field}>
            <label>Peak ranges (HH:MM, [start, end))</label>
            {ranges.map((range, index) => (
              <div key={index} className={styles.rangeRow}>
                <Input
                  value={range.start}
                  onChange={(event) => updateRange(index, { start: event.target.value })}
                  placeholder="09:00"
                  disabled={saving}
                />
                <span>→</span>
                <Input
                  value={range.end}
                  onChange={(event) => updateRange(index, { end: event.target.value })}
                  placeholder="12:00"
                  disabled={saving}
                />
                <Button
                  variant="danger"
                  size="sm"
                  appearance="action"
                  onClick={() => setRanges((current) => current.filter((_, i) => i !== index))}
                  disabled={saving || ranges.length <= 1}
                >
                  {t('common.delete')}
                </Button>
              </div>
            ))}
            <Button
              variant="secondary"
              size="sm"
              appearance="action"
              onClick={() => setRanges((current) => [...current, { start: '', end: '' }])}
              disabled={saving}
            >
              Add range
            </Button>
          </div>
        </div>
      )}
    </Modal>
  )
}
