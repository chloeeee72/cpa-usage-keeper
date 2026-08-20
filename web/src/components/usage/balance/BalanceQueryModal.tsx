import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/Button'
import { IconRefreshCw } from '@/components/ui/icons'
import { LoadingSpinner } from '@/components/ui/LoadingSpinner'
import { Modal } from '@/components/ui/Modal'
import { ApiError, fetchBalanceSummary } from '@/lib/api'
import type { BalanceQueryItem, BalanceQueryResponse } from '@/lib/types'
import { useThemeStore } from '@/stores/useThemeStore'
import { BalanceCharts } from './BalanceCharts'
import { BalanceItemsTable } from './BalanceItemsTable'
import { BalanceSummaryCards } from './BalanceSummaryCards'
import styles from './BalanceQueryModal.module.scss'
import { recomputeBalanceTotals } from './balanceChartConfig'

interface BalanceQueryModalProps {
  open: boolean
  onClose: () => void
  onAuthRequired?: () => void
}

export function BalanceQueryModal({ open, onClose, onAuthRequired }: BalanceQueryModalProps) {
  const { t } = useTranslation()
  const isDark = useThemeStore((state) => state.resolvedTheme === 'dark')
  const [data, setData] = useState<BalanceQueryResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [refreshError, setRefreshError] = useState('')
  const [refreshingIdentityId, setRefreshingIdentityId] = useState<string | null>(null)
  const requestControllerRef = useRef<AbortController | null>(null)

  const loadAll = useCallback(async (signal?: AbortSignal) => {
    setLoading(true)
    setRefreshError('')
    try {
      const response = await fetchBalanceSummary({ signal })
      setData(response)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (!open) {
      requestControllerRef.current?.abort()
      requestControllerRef.current = null
      setData(null)
      setRefreshError('')
      setRefreshing(false)
      setRefreshingIdentityId(null)
      return
    }
    const controller = new AbortController()
    requestControllerRef.current = controller
    void loadAll(controller.signal).catch((error: unknown) => {
      if (controller.signal.aborted) return
      if (error instanceof ApiError && error.status === 401) {
        onAuthRequired?.()
        return
      }
      setRefreshError(error instanceof Error ? error.message : t('usage_stats.balance_query_error'))
    })
    return () => {
      controller.abort()
      if (requestControllerRef.current === controller) {
        requestControllerRef.current = null
      }
    }
  }, [loadAll, onAuthRequired, open, t])

  const handleRefreshAll = useCallback(async () => {
    setRefreshing(true)
    setRefreshError('')
    try {
      const response = await fetchBalanceSummary()
      setData(response)
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        onAuthRequired?.()
        return
      }
      setRefreshError(error instanceof Error ? error.message : t('usage_stats.balance_query_refresh_failed'))
    } finally {
      setRefreshing(false)
    }
  }, [onAuthRequired, t])

  const handleRefreshItem = useCallback(async (identityId: string) => {
    setRefreshingIdentityId(identityId)
    try {
      const response = await fetchBalanceSummary({ identityId })
      const nextItem = response.items[0]
      if (!nextItem) return
      setData((current) => {
        if (!current) return response
        const items = current.items.map((item) => (item.identity_id === identityId ? nextItem : item))
        return {
          ...current,
          items,
          totals: recomputeBalanceTotals(items),
          succeeded_count: items.filter((item) => item.summary).length,
          failed_count: items.filter((item) => item.error).length,
        }
      })
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        onAuthRequired?.()
        return
      }
      const message = error instanceof Error ? error.message : t('usage_stats.balance_query_error')
      setData((current) => {
        if (!current) return current
        const items: BalanceQueryItem[] = current.items.map((item) => (
          item.identity_id === identityId
            ? { ...item, summary: undefined, error: message }
            : item
        ))
        return {
          ...current,
          items,
          totals: recomputeBalanceTotals(items),
          succeeded_count: items.filter((item) => item.summary).length,
          failed_count: items.filter((item) => item.error).length,
        }
      })
    } finally {
      setRefreshingIdentityId(null)
    }
  }, [onAuthRequired, t])

  const hasData = data !== null
  const busy = loading || refreshing

  return (
    <Modal
      open={open}
      onClose={onClose}
      width="min(920px, 100%)"
      title={t('usage_stats.balance_query_modal_title')}
      footer={(
        <Button type="button" variant="primary" onClick={() => void handleRefreshAll()} loading={busy} disabled={busy || refreshingIdentityId !== null}>
          <IconRefreshCw size={14} />
          {t('usage_stats.balance_query_refresh')}
        </Button>
      )}
    >
      {loading && (
        <div className={styles.queryState}>
          <LoadingSpinner size={28} />
        </div>
      )}
      {!loading && hasData && data.configured_count === 0 && (
        <div className={styles.queryState}>{t('usage_stats.balance_query_empty')}</div>
      )}
      {!loading && hasData && data.configured_count > 0 && (
        <div className={styles.queryContent}>
          {refreshError && <div className="error-box">{refreshError}</div>}
          <BalanceSummaryCards totals={data.totals} />
          <BalanceCharts items={data.items} isDark={isDark} />
          <BalanceItemsTable
            items={data.items}
            refreshingIdentityId={refreshingIdentityId}
            onRefreshItem={(identityId) => void handleRefreshItem(identityId)}
          />
        </div>
      )}
    </Modal>
  )
}
