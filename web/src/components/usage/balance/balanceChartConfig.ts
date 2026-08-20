import type { ChartData, ChartOptions } from 'chart.js'
import type { BalanceQueryItem, TokenRhythmBalanceTotals } from '@/lib/types'

export function formatCny(value: number): string {
  return new Intl.NumberFormat('zh-CN', {
    style: 'currency',
    currency: 'CNY',
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(Number.isFinite(value) ? value : 0)
}

export function formatCnyCompact(value: number): string {
  return new Intl.NumberFormat('zh-CN', {
    style: 'currency',
    currency: 'CNY',
    notation: 'compact',
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
  }).format(Number.isFinite(value) ? value : 0)
}

export function recomputeBalanceTotals(items: BalanceQueryItem[]): TokenRhythmBalanceTotals {
  const totals: TokenRhythmBalanceTotals = {
    balance_cny: 0,
    available_balance_cny: 0,
    frozen_balance_cny: 0,
    expiring_balance_cny: 0,
    calls: 0,
    cost_cny: 0,
  }
  for (const item of items) {
    if (!item.summary) continue
    totals.balance_cny += item.summary.balanceCny
    totals.available_balance_cny += item.summary.availableBalanceCny
    totals.frozen_balance_cny += item.summary.frozenBalanceCny
    totals.expiring_balance_cny += item.summary.expiringBalanceCny
    totals.calls += item.summary.calls
    totals.cost_cny += item.summary.costCny
  }
  return totals
}

const SUCCESS_ITEMS = (items: BalanceQueryItem[]) => items.filter((item) => item.summary)

function palette(isDark: boolean): { border: string; background: string[] } {
  if (isDark) {
    return {
      border: 'rgba(148, 163, 184, 0.9)',
      background: ['#60a5fa', '#34d399', '#fbbf24', '#f472b6', '#a78bfa', '#22d3ee', '#fb923c'],
    }
  }
  return {
    border: 'rgba(100, 116, 139, 0.9)',
    background: ['#2563eb', '#059669', '#d97706', '#db2777', '#7c3aed', '#0891b2', '#ea580c'],
  }
}

export function buildBalanceBarChartData(items: BalanceQueryItem[], isDark: boolean): ChartData<'bar'> {
  const successful = SUCCESS_ITEMS(items)
  return {
    labels: successful.map((item) => item.display_name),
    datasets: [
      {
        label: 'balanceCny',
        data: successful.map((item) => item.summary?.balanceCny ?? 0),
        backgroundColor: palette(isDark).background[0],
        borderRadius: 4,
      },
    ],
  }
}

export function buildBalanceDoughnutChartData(items: BalanceQueryItem[], isDark: boolean): ChartData<'doughnut'> {
  const successful = SUCCESS_ITEMS(items)
  const colors = palette(isDark).background
  return {
    labels: successful.map((item) => item.display_name),
    datasets: [
      {
        data: successful.map((item) => item.summary?.balanceCny ?? 0),
        backgroundColor: successful.map((_, index) => colors[index % colors.length]),
        borderWidth: 1,
      },
    ],
  }
}

export function buildBalanceBarChartOptions(isDark: boolean): ChartOptions<'bar'> {
  return {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: { display: false },
      tooltip: {
        callbacks: {
          label: (context) => formatCny(Number(context.parsed.y ?? 0)),
        },
      },
    },
    scales: {
      x: {
        ticks: { color: isDark ? '#94a3b8' : '#475569', maxRotation: 40, minRotation: 0 },
        grid: { display: false },
      },
      y: {
        ticks: {
          color: isDark ? '#94a3b8' : '#475569',
          callback: (value) => formatCnyCompact(Number(value)),
        },
        grid: { color: isDark ? 'rgba(148, 163, 184, 0.16)' : 'rgba(100, 116, 139, 0.14)' },
      },
    },
  }
}

export function buildBalanceDoughnutChartOptions(isDark: boolean): ChartOptions<'doughnut'> {
  return {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: {
        position: 'right',
        labels: { color: isDark ? '#cbd5e1' : '#334155' },
      },
      tooltip: {
        callbacks: {
          label: (context) => {
            const value = Number(context.parsed ?? 0)
            const total = (context.dataset.data as number[]).reduce((sum, item) => sum + Number(item), 0)
            const percent = total > 0 ? (value / total) * 100 : 0
            return `${context.label}: ${formatCny(value)} (${percent.toFixed(1)}%)`
          },
        },
      },
    },
  }
}
