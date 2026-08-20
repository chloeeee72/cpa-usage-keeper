import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Bar, Doughnut } from 'react-chartjs-2'
import type { BalanceQueryItem } from '@/lib/types'
import styles from './BalanceQueryModal.module.scss'
import { buildBalanceBarChartData, buildBalanceBarChartOptions, buildBalanceDoughnutChartData, buildBalanceDoughnutChartOptions } from './balanceChartConfig'

interface BalanceChartsProps {
  items: BalanceQueryItem[]
  isDark: boolean
}

export function BalanceCharts({ items, isDark }: BalanceChartsProps) {
  const { t } = useTranslation()
  const barData = useMemo(() => buildBalanceBarChartData(items, isDark), [items, isDark])
  const doughnutData = useMemo(() => buildBalanceDoughnutChartData(items, isDark), [items, isDark])
  const barOptions = useMemo(() => buildBalanceBarChartOptions(isDark), [isDark])
  const doughnutOptions = useMemo(() => buildBalanceDoughnutChartOptions(isDark), [isDark])

  return (
    <div className={styles.charts}>
      <div className={styles.chartPanel}>
        <div className={styles.chartTitle}>{t('usage_stats.balance_query_total')}</div>
        <div className={`${styles.chartCanvas} ${styles.chartCanvasBar}`}>
          <Bar data={barData} options={barOptions} />
        </div>
      </div>
      <div className={styles.chartPanel}>
        <div className={styles.chartTitle}>{t('usage_stats.balance_query_available')}</div>
        <div className={`${styles.chartCanvas} ${styles.chartCanvasDoughnut}`}>
          <Doughnut data={doughnutData} options={doughnutOptions} />
        </div>
      </div>
    </div>
  )
}
