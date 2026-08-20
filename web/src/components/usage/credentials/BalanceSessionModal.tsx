import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/Button'
import { IconEye, IconEyeOff } from '@/components/ui/icons'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'

interface BalanceSessionModalProps {
  open: boolean
  identityId: string | null
  displayName: string
  apiKeyMasked?: string
  configured: boolean
  saving: boolean
  onClose: () => void
  onSave: (identityId: string, session: string | null) => Promise<void>
}

export function BalanceSessionModal({ open, identityId, displayName, apiKeyMasked, configured, saving, onClose, onSave }: BalanceSessionModalProps) {
  const { t } = useTranslation()
  const [session, setSession] = useState('')
  const [visible, setVisible] = useState(false)
  const [error, setError] = useState('')

  const handleSave = useCallback(async () => {
    if (!identityId) return
    const trimmed = session.trim()
    if (trimmed !== '' && (trimmed.length < 8 || /[\r\n\t]/.test(trimmed))) {
      setError(t('usage_stats.credentials_balance_session_hint'))
      return
    }
    setError('')
    try {
      await onSave(identityId, trimmed === '' ? null : trimmed)
      onClose()
    } catch {
      // 保存失败提示由 useCredentialsTabData 统一触发。
    }
  }, [identityId, onClose, onSave, session, t])

  const handleClear = useCallback(async () => {
    if (!identityId) return
    setError('')
    try {
      await onSave(identityId, null)
      onClose()
    } catch {
      // 保存失败提示由 useCredentialsTabData 统一触发。
    }
  }, [identityId, onClose, onSave])

  return (
    <Modal
      open={open}
      onClose={onClose}
      width={440}
      title={t('usage_stats.credentials_balance_session_configure')}
      footer={(
        <>
          {configured && (
            <Button type="button" variant="ghost" onClick={() => void handleClear()} disabled={saving}>
              {t('usage_stats.credentials_balance_session_clear')}
            </Button>
          )}
          <Button type="button" variant="primary" onClick={() => void handleSave()} loading={saving}>
            {t('common.save')}
          </Button>
        </>
      )}
    >
      <div className="form-group">
        <p className="hint" style={{ margin: 0 }}>
          {t('usage_stats.credentials_balance_session_hint')}
        </p>
      </div>
      {apiKeyMasked && (
        <Input
          label={t('usage_stats.api_key_settings_display_key')}
          value={apiKeyMasked}
          readOnly
        />
      )}
      <Input
        label={displayName}
        type={visible ? 'text' : 'password'}
        value={session}
        onChange={(event) => setSession(event.target.value)}
        placeholder="tr_session"
        autoComplete="off"
        spellCheck={false}
        error={error}
        rightElement={(
          <button
            type="button"
            className="btn btn-ghost btn-sm"
            onClick={() => setVisible((current) => !current)}
            aria-label={visible ? 'Hide session' : 'Show session'}
          >
            {visible ? <IconEyeOff size={14} /> : <IconEye size={14} />}
          </button>
        )}
      />
    </Modal>
  )
}
