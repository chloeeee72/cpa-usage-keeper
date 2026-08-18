import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import {
  fetchPricing,
  fetchPricingRules,
  replacePricingRules,
  updatePricing,
} from '@/lib/api'
import type { PricingStyle, ReplacePricingRuleInput } from '@/lib/types'
import styles from './PeakOffPeakPriceModal.module.scss'

export interface PeakOffPeakPriceModalProps {
  open: boolean
  model: string
  onClose: () => void
  onNotice?: (kind: 'success' | 'info' | 'error', message: string) => void
}

interface PeakPriceDraft {
  style: PricingStyle
  prompt: string
  completion: string
  cacheRead: string
  cacheWrite: string
}

const emptyDraft = (): PeakPriceDraft => ({
  style: 'openai',
  prompt: '',
  completion: '',
  cacheRead: '',
  cacheWrite: '',
})

const toInputValue = (value: number | undefined): string => (value == null ? '' : String(value))

export function PeakOffPeakPriceModal({ open, model, onClose, onNotice }: PeakOffPeakPriceModalProps) {
  const { t } = useTranslation()
  const [peak, setPeak] = useState<PeakPriceDraft>(emptyDraft())
  const [offPeakMultiplier, setOffPeakMultiplier] = useState('1')
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!open || !model) return
    let cancelled = false
    setLoading(true)
    setPeak(emptyDraft())
    setOffPeakMultiplier('1')

    Promise.all([fetchPricing(), fetchPricingRules(model)])
      .then(([pricingResponse, rulesResponse]) => {
        if (cancelled) return
        const entry = pricingResponse.pricing.find((item) => item.model === model)
        if (entry) {
          setPeak({
            style: entry.pricing_style || 'openai',
            prompt: toInputValue(entry.prompt_price_per_1m),
            completion: toInputValue(entry.completion_price_per_1m),
            cacheRead: toInputValue(entry.cache_read_price_per_1m),
            cacheWrite: toInputValue(entry.cache_write_price_per_1m),
          })
        }
        const offPeakRule = rulesResponse.rules.find(
          (rule) => rule.key === 'pricing_period' && rule.value === 'off_peak',
        )
        setOffPeakMultiplier(offPeakRule ? toInputValue(offPeakRule.multiplier) : '1')
      })
      .catch(() => {
        if (!cancelled) onNotice?.('error', 'Failed to load peak/off-peak pricing')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [model, open, onNotice])

  const multiplier = Number(offPeakMultiplier)
  const multiplierValid = offPeakMultiplier.trim() !== '' && Number.isFinite(multiplier) && multiplier >= 0

  const save = async () => {
    if (saving || !multiplierValid) return
    setSaving(true)
    try {
      await updatePricing(model, {
        pricing_style: peak.style,
        prompt_price_per_1m: Number(peak.prompt) || 0,
        completion_price_per_1m: Number(peak.completion) || 0,
        cache_read_price_per_1m: Number(peak.cacheRead) || 0,
        cache_write_price_per_1m: Number(peak.cacheWrite) || 0,
        price_multiplier: 1,
      })

      const rules: ReplacePricingRuleInput[] =
        multiplier === 1
          ? []
          : [
              { key: 'pricing_period', value: 'peak', multiplier: 1 },
              { key: 'pricing_period', value: 'off_peak', multiplier },
            ]
      await replacePricingRules({ model, rules })
      onNotice?.('success', 'Peak/off-peak pricing updated')
      onClose()
    } catch {
      onNotice?.('error', 'Failed to update peak/off-peak pricing')
    } finally {
      setSaving(false)
    }
  }

  const renderPeakField = (label: string, value: string, onChange: (value: string) => void) => (
    <div className={styles.field}>
      <label>{label}</label>
      <Input
        type="number"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder="0.00"
        step="0.0001"
        disabled={saving}
      />
    </div>
  )

  return (
    <Modal
      open={open}
      title={`Peak / off-peak · ${model}`}
      onClose={onClose}
      closeDisabled={saving}
      footer={
        <div className={styles.footer}>
          <Button variant="secondary" appearance="action" onClick={onClose} disabled={saving}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" appearance="action" onClick={() => void save()} loading={saving} disabled={!multiplierValid}>
            {t('common.save')}
          </Button>
        </div>
      }
      width={560}
    >
      {loading ? (
        <div className={styles.loading}>{t('common.loading')}</div>
      ) : (
        <div className={styles.body}>
          <div className={styles.section}>
            <div className={styles.sectionTitle}>Peak price (base price)</div>
            <div className={styles.grid}>
              {renderPeakField('Input ¥/1M', peak.prompt, (value) => setPeak((current) => ({ ...current, prompt: value })))}
              {renderPeakField('Output ¥/1M', peak.completion, (value) => setPeak((current) => ({ ...current, completion: value })))}
              {renderPeakField('Cache read ¥/1M', peak.cacheRead, (value) => setPeak((current) => ({ ...current, cacheRead: value })))}
              {renderPeakField('Cache write ¥/1M', peak.cacheWrite, (value) => setPeak((current) => ({ ...current, cacheWrite: value })))}
            </div>
          </div>

          <div className={styles.section}>
            <div className={styles.sectionTitle}>Off-peak</div>
            <div className={styles.grid}>
              <div className={styles.field}>
                <label>Off-peak multiplier</label>
                <Input
                  type="number"
                  value={offPeakMultiplier}
                  onChange={(event) => setOffPeakMultiplier(event.target.value)}
                  placeholder="0.5"
                  step="0.01"
                  min="0"
                  disabled={saving}
                />
              </div>
            </div>
            {multiplierValid && (
              <div className={styles.preview}>
                Off-peak input ≈ {(Number(peak.prompt) || 0) * multiplier} ¥/1M
                {' · '}
                output ≈ {(Number(peak.completion) || 0) * multiplier} ¥/1M
              </div>
            )}
          </div>
        </div>
      )}
    </Modal>
  )
}
