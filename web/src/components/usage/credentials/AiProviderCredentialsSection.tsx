import { useCallback, useMemo, useState, type CSSProperties } from 'react'
import { useTranslation } from 'react-i18next'
import styles from './CredentialSections.module.scss'
import type { AiProviderCredentialRow } from './credentialViewModels'
import type { UsageIdentityPageSort } from '@/lib/api'
import { Button } from '@/components/ui/Button'
import { IconKey } from '@/components/ui/icons'
import { BalanceSessionModal } from './BalanceSessionModal'
import { CredentialAliasEditor, isCredentialAliasEditorDisabled } from './CredentialAliasEditor'
import { CredentialColumnResizer } from './CredentialColumnResizer'
import { CredentialHealthPanel } from './CredentialHealthPanel'
import { CredentialPriorityBadge, CredentialRowShell, CredentialSectionShell, CredentialTableHeader, CredentialsPagination, MetricPill, RequestMetric, TonePercent, cacheReadRateTone, formatCredentialNumber, successRateTone } from './CredentialSectionShell'
import { formatUsageIdentityTotalCost } from './credentialViewModels'
import { ProviderBrandIcon } from '@/components/ProviderBrandIcon'

const AI_PROVIDER_COLUMN_MIN_NAME = 200
const AI_PROVIDER_COLUMN_MAX_NAME = 520
const AI_PROVIDER_COLUMN_DEFAULT_NAME = 240
const AI_PROVIDER_COLUMN_MIN_METRICS = 500
const AI_PROVIDER_COLUMN_MAX_METRICS = 720
const AI_PROVIDER_COLUMN_DEFAULT_METRICS = 500
const AI_PROVIDER_COLUMN_MIN_SIDE = 420
const AI_PROVIDER_COLUMN_GAP = 18
const AI_PROVIDER_TABLE_PADDING = 22

function clampColumnWidth(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value))
}

interface AiProviderCredentialsSectionProps {
  rows: AiProviderCredentialRow[]
  total: number
  page: number
  totalPages: number
  pageSize: number
  sort: UsageIdentityPageSort
  loading: boolean
  aliasSavingId?: string
  balanceSessionSavingId?: string
  onSaveAlias?: (id: string, alias: string) => Promise<void>
  onSaveBalanceSession?: (id: string, session: string | null) => Promise<void>
  onPageChange: (page: number) => void
  onPageSizeChange: (pageSize: number) => void
  onSortChange: (sort: UsageIdentityPageSort) => void
}

export function AiProviderCredentialsSection({ rows, total, page, totalPages, pageSize, sort, loading, aliasSavingId, balanceSessionSavingId, onSaveAlias, onSaveBalanceSession, onPageChange, onPageSizeChange, onSortChange }: AiProviderCredentialsSectionProps) {
  const { t } = useTranslation()
  const [sessionEditingRow, setSessionEditingRow] = useState<AiProviderCredentialRow | null>(null)
  const [nameColumnWidth, setNameColumnWidth] = useState(AI_PROVIDER_COLUMN_DEFAULT_NAME)
  const [metricsColumnWidth, setMetricsColumnWidth] = useState(AI_PROVIDER_COLUMN_DEFAULT_METRICS)

  const gridTemplateColumns = useMemo(
    () => `${nameColumnWidth}px ${metricsColumnWidth}px minmax(${AI_PROVIDER_COLUMN_MIN_SIDE}px, 1fr)`,
    [metricsColumnWidth, nameColumnWidth],
  )
  const sectionStyle = useMemo(
    () => ({ '--ai-provider-grid-columns': gridTemplateColumns }) as CSSProperties,
    [gridTemplateColumns],
  )

  const handleResizeNameColumn = useCallback((deltaX: number) => {
    setNameColumnWidth((current) => clampColumnWidth(current + deltaX, AI_PROVIDER_COLUMN_MIN_NAME, AI_PROVIDER_COLUMN_MAX_NAME))
  }, [])

  const handleResizeMetricsColumn = useCallback((deltaX: number) => {
    setMetricsColumnWidth((current) => clampColumnWidth(current + deltaX, AI_PROVIDER_COLUMN_MIN_METRICS, AI_PROVIDER_COLUMN_MAX_METRICS))
  }, [])

  const nameResizerLeft = AI_PROVIDER_TABLE_PADDING + nameColumnWidth + AI_PROVIDER_COLUMN_GAP / 2 - 6
  const metricsResizerLeft = AI_PROVIDER_TABLE_PADDING + nameColumnWidth + AI_PROVIDER_COLUMN_GAP + metricsColumnWidth + AI_PROVIDER_COLUMN_GAP / 2 - 6

  return (
    <CredentialSectionShell
      title={t('usage_stats.credentials_ai_providers_title')}
      subtitle={t('usage_stats.credentials_ai_providers_subtitle')}
      countLabel={t('usage_stats.credentials_count', { count: total })}
      style={sectionStyle}
    >
      {loading && rows.length === 0 && <div className={styles.credentialEmptyState}>{t('common.loading')}</div>}
      {!loading && rows.length === 0 && <div className={styles.credentialEmptyState}>{t('usage_stats.credentials_ai_providers_empty')}</div>}
      {rows.length > 0 && (
        <CredentialTableHeader
          rowClassName={styles.aiProviderCredentialRow}
          nameLabel={t('usage_stats.credentials_column_name')}
          totalRequestsLabel={t('usage_stats.total_requests')}
          successRateLabel={t('usage_stats.success_rate')}
          totalTokensLabel={t('usage_stats.total_tokens')}
          totalCostLabel={t('usage_stats.credentials_column_total_cost')}
          cacheReadRateLabel={t('usage_stats.cache_rate')}
          sideLabel={t('usage_stats.credentials_column_health')}
          resizers={(
            <>
              <CredentialColumnResizer
                ariaLabel={t('usage_stats.credentials_resize_name_column')}
                style={{ left: nameResizerLeft }}
                onResize={handleResizeNameColumn}
              />
              <CredentialColumnResizer
                ariaLabel={t('usage_stats.credentials_resize_metrics_column')}
                style={{ left: metricsResizerLeft }}
                onResize={handleResizeMetricsColumn}
              />
            </>
          )}
        />
      )}
      {rows.map((row) => (
        <CredentialRowShell
          key={row.identity.id || row.identity.identity}
          icon={<ProviderBrandIcon providerType={row.identity.type} size={30} ariaLabel={row.typeLabel} />}
          title={onSaveAlias ? (
            <CredentialAliasEditor
              identityId={row.identity.id}
              displayName={row.displayName}
              alias={row.identity.alias}
              saving={aliasSavingId === row.identity.id}
              disabled={isCredentialAliasEditorDisabled(row.identity.id, row.identity.is_deleted, aliasSavingId)}
              onSaveAlias={onSaveAlias}
            />
          ) : row.displayName}
          subtitle={row.priorityLabel ? (
            <span className={styles.credentialIdentityBadges}>
              <CredentialPriorityBadge>{row.priorityLabel}</CredentialPriorityBadge>
            </span>
          ) : undefined}
          badges={null}
          metrics={(
            <>
              <MetricPill value={<RequestMetric total={row.totalRequests} success={row.successCount} failure={row.failureCount} />} />
              <MetricPill value={<TonePercent value={row.successRate} tone={successRateTone(row.successRate)} />} />
              <MetricPill value={formatCredentialNumber(row.totalTokens)} />
              <MetricPill value={formatUsageIdentityTotalCost(row.totalCostUsd, row.costAvailable)} />
              <MetricPill value={<TonePercent value={row.cacheReadRate} tone={cacheReadRateTone(row.cacheReadRate)} />} />
            </>
          )}
          side={(
            <div className={styles.aiProviderCredentialSide}>
              <CredentialHealthPanel displayName={row.displayName} health={row.credentialHealth} lastUsedAt={row.lastUsedText} statsUpdatedAt={row.statsUpdatedText} />
              {row.identity.balance_session_supported && onSaveBalanceSession && (
                <div className={styles.credentialBalanceSessionAction}>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => setSessionEditingRow(row)}
                    disabled={balanceSessionSavingId === row.identity.id}
                  >
                    <IconKey size={12} />
                    {row.identity.balance_session_configured
                      ? t('usage_stats.credentials_balance_session_configured')
                      : t('usage_stats.credentials_balance_session_configure')}
                  </Button>
                </div>
              )}
            </div>
          )}
          rowClassName={styles.aiProviderCredentialRow}
        />
      ))}
      <CredentialsPagination
        page={page}
        total={total}
        totalPages={totalPages}
        pageSize={pageSize}
        sortValue={sort}
        sortLabel={t('usage_stats.credentials_sort_label')}
        sortOptions={[
          { value: 'priority', label: t('usage_stats.credentials_sort_priority') },
          { value: 'total_requests', label: t('usage_stats.credentials_sort_total_requests') },
          { value: 'total_tokens', label: t('usage_stats.credentials_sort_total_tokens') },
          { value: 'last_used_at', label: t('usage_stats.credentials_sort_last_used') },
        ]}
        previousLabel={t('usage_stats.previous_page')}
        nextLabel={t('usage_stats.next_page')}
        rowsPerPageLabel={t('usage_stats.rows_per_page')}
        onPageChange={onPageChange}
        onPageSizeChange={onPageSizeChange}
        onSortChange={(nextSort) => onSortChange(nextSort as UsageIdentityPageSort)}
      />
      <BalanceSessionModal
        key={sessionEditingRow?.identity.id ?? 'closed'}
        open={sessionEditingRow !== null}
        identityId={sessionEditingRow?.identity.id ?? null}
        displayName={sessionEditingRow?.displayName ?? ''}
        apiKeyMasked={sessionEditingRow?.identity.api_key_masked}
        configured={Boolean(sessionEditingRow?.identity.balance_session_configured)}
        saving={sessionEditingRow ? balanceSessionSavingId === sessionEditingRow.identity.id : false}
        onClose={() => setSessionEditingRow(null)}
        onSave={async (id, session) => {
          await onSaveBalanceSession?.(id, session)
        }}
      />
    </CredentialSectionShell>
  )
}
