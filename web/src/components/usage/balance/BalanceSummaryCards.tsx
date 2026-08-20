import { useTranslation } from 'react-i18next'
import { Card } from '@/components/ui/Card'
import type { TokenRhythmBalanceTotals } from '@/lib/types'
import styles from './BalanceQueryModal.module.scss'
import { formatCny } from './balanceChartConfig'

interface BalanceSummaryCardsProps {
  totals: TokenRhythmBalanceTotals
}

export function BalanceSummaryCards({ totals }: BalanceSummaryCardsProps) {
  const { t } = useTranslation()
  const cards = [
    { label: t('usage_stats.balance_query_total'), value: formatCny(totals.balance_cny) },
    { label: t('usage_stats.balance_query_available'), value: formatCny(totals.available_balance_cny) },
    { label: t('usage_stats.balance_query_frozen'), value: formatCny(totals.frozen_balance_cny) },
    { label: t('usage_stats.balance_query_expiring'), value: formatCny(totals.expiring_balance_cny) },
    { label: t('usage_stats.balance_query_calls'), value: String(totals.calls) },
    { label: t('usage_stats.balance_query_cost'), value: formatCny(totals.cost_cny) },
  ]

  return (
    <div className={styles.summaryCards}>
      {cards.map((card) => (
        <Card key={card.label} className={styles.summaryCard}>
          <div className={styles.summaryCardLabel}>{card.label}</div>
          <div className={styles.summaryCardValue}>{card.value}</div>
        </Card>
      ))}
    </div>
  )
}
