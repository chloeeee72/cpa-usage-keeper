import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/Button'
import { IconRefreshCw } from '@/components/ui/icons'
import type { BalanceQueryItem } from '@/lib/types'
import styles from './BalanceQueryModal.module.scss'
import { formatCny } from './balanceChartConfig'

interface BalanceItemsTableProps {
  items: BalanceQueryItem[]
  refreshingIdentityId: string | null
  onRefreshItem: (identityId: string) => void
}

export function BalanceItemsTable({ items, refreshingIdentityId, onRefreshItem }: BalanceItemsTableProps) {
  const { t } = useTranslation()

  return (
    <div className={styles.itemsTableWrap}>
      <table className={styles.itemsTable}>
        <thead>
          <tr>
            <th>{t('usage_stats.credentials_column_name')}</th>
            <th>{t('usage_stats.balance_query_total')}</th>
            <th>{t('usage_stats.balance_query_available')}</th>
            <th>{t('usage_stats.balance_query_frozen')}</th>
            <th>{t('usage_stats.balance_query_expiring')}</th>
            <th>{t('usage_stats.balance_query_next_expiry')}</th>
            <th>{t('usage_stats.balance_query_calls')}</th>
            <th>{t('usage_stats.balance_query_cost')}</th>
            <th>{t('usage_stats.credentials_refresh_status')}</th>
            <th aria-label={t('usage_stats.balance_query_item_refresh')} />
          </tr>
        </thead>
        <tbody>
          {items.map((item) => {
            const refreshing = refreshingIdentityId === item.identity_id
            const summary = item.summary
            return (
              <tr key={item.identity_id} className={item.error ? styles.itemsRowError : undefined}>
                <td>{item.display_name}</td>
                <td>{summary ? formatCny(summary.balanceCny) : '-'}</td>
                <td>{summary ? formatCny(summary.availableBalanceCny) : '-'}</td>
                <td>{summary ? formatCny(summary.frozenBalanceCny) : '-'}</td>
                <td>{summary ? formatCny(summary.expiringBalanceCny) : '-'}</td>
                <td>{summary?.nextExpiryAt || '-'}</td>
                <td>{summary ? String(summary.calls) : '-'}</td>
                <td>{summary ? formatCny(summary.costCny) : '-'}</td>
                <td>{item.error ? t('usage_stats.balance_query_item_error', { error: item.error }) : t('usage_stats.credentials_refresh_status_completed')}</td>
                <td>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    loading={refreshing}
                    disabled={refreshing}
                    onClick={() => onRefreshItem(item.identity_id)}
                  >
                    <IconRefreshCw size={12} />
                    {t('usage_stats.balance_query_item_refresh')}
                  </Button>
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
