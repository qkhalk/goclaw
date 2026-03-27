import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { ChannelFields } from './channel-field-renderer'
import { configSchema, ESSENTIAL_CONFIG_KEYS, NETWORK_KEYS, LIMITS_KEYS, STREAMING_KEYS, BEHAVIOR_KEYS, ACCESS_KEYS } from './channel-schemas'
import type { ChannelInstanceData } from '../../../types/channel'

interface ChannelAdvancedDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  instance: ChannelInstanceData
  onUpdate: (updates: Record<string, unknown>) => Promise<void>
}

const ESSENTIAL_CONFIG_KEYS_SET = new Set(['dm_policy', 'group_policy', 'require_mention', 'mention_mode'])

function getAdvancedFields(channelType: string) {
  const allFields = configSchema[channelType] ?? []
  const advanced = allFields.filter((f) => !ESSENTIAL_CONFIG_KEYS_SET.has(f.key))
  return {
    network: advanced.filter((f) => NETWORK_KEYS.has(f.key)),
    limits: advanced.filter((f) => LIMITS_KEYS.has(f.key)),
    streaming: advanced.filter((f) => STREAMING_KEYS.has(f.key)),
    behavior: advanced.filter((f) => BEHAVIOR_KEYS.has(f.key)),
    access: advanced.filter((f) => ACCESS_KEYS.has(f.key)),
  }
}

function deriveInitialValues(instance: ChannelInstanceData): Record<string, unknown> {
  const config = (instance.config ?? {}) as Record<string, unknown>
  const { groups: _groups, ...rest } = config
  return Object.fromEntries(
    Object.entries(rest).filter(([k]) => !ESSENTIAL_CONFIG_KEYS_SET.has(k))
  )
}

export function ChannelAdvancedDialog({ open, onOpenChange, instance, onUpdate }: ChannelAdvancedDialogProps) {
  const { t } = useTranslation('channels')
  const [values, setValues] = useState<Record<string, unknown>>({})
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (open) {
      setValues(deriveInitialValues(instance))
      setError('')
    }
  }, [open, instance])

  if (!open) return null

  const fields = getAdvancedFields(instance.channel_type)
  const essentialKeys = ESSENTIAL_CONFIG_KEYS[instance.channel_type] ?? ESSENTIAL_CONFIG_KEYS['_default']

  const handleChange = (key: string, value: unknown) => {
    setValues((prev) => ({ ...prev, [key]: value }))
  }

  const handleSave = async () => {
    setSaving(true)
    setError('')
    try {
      const existingConfig = (instance.config ?? {}) as Record<string, unknown>
      // Preserve essential keys + groups key
      const essential = Object.fromEntries(
        Object.entries(existingConfig).filter(([k]) => essentialKeys.includes(k) || k === 'groups')
      )
      await onUpdate({ ...essential, ...values })
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('advanced.saveFailed'))
    } finally {
      setSaving(false)
    }
  }

  const groups = [
    { key: 'network', label: t('advanced.network'), fields: fields.network },
    { key: 'limits', label: t('advanced.limits'), fields: fields.limits },
    { key: 'streaming', label: t('advanced.streaming'), fields: fields.streaming },
    { key: 'behavior', label: t('advanced.behavior'), fields: fields.behavior },
    { key: 'access', label: t('advanced.access'), fields: fields.access },
  ].filter((g) => g.fields.length > 0)

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-sm">
      <div className="bg-surface-secondary border border-border rounded-xl shadow-xl max-w-lg w-full mx-4 flex flex-col" style={{ maxHeight: '85vh' }}>
        {/* Header */}
        <div className="flex items-center justify-between border-b border-border px-5 py-4 shrink-0">
          <h3 className="text-sm font-semibold text-text-primary">{t('advanced.title')}</h3>
          <button onClick={() => onOpenChange(false)} className="p-1 text-text-muted hover:text-text-primary transition-colors cursor-pointer">
            <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
              <path d="M18 6 6 18" /><path d="m6 6 12 12" />
            </svg>
          </button>
        </div>

        {/* Scrollable body */}
        <div className="flex-1 overflow-y-auto overscroll-contain px-5 py-4 space-y-5">
          {groups.length === 0 ? (
            <p className="text-sm text-text-muted text-center py-6">{t('advanced.noFields')}</p>
          ) : (
            groups.map((g) => (
              <div key={g.key}>
                <p className="text-xs font-semibold text-text-secondary uppercase tracking-wider mb-3">{g.label}</p>
                <ChannelFields
                  fields={g.fields}
                  values={values}
                  onChange={handleChange}
                  idPrefix={`adv-${g.key}`}
                  isEdit
                />
              </div>
            ))
          )}
          {error && <p className="text-xs text-error">{error}</p>}
        </div>

        {/* Footer */}
        <div className="flex justify-end gap-2 border-t border-border px-5 py-3 shrink-0">
          <button
            onClick={() => onOpenChange(false)}
            className="px-3 py-1.5 text-sm text-text-secondary hover:text-text-primary transition-colors cursor-pointer"
          >
            {t('common.cancel')}
          </button>
          <button
            onClick={handleSave}
            disabled={saving}
            className="px-4 py-1.5 bg-accent text-white text-sm rounded-lg disabled:opacity-50 cursor-pointer hover:opacity-90 transition-opacity"
          >
            {saving ? t('common.saving') : t('common.save')}
          </button>
        </div>
      </div>
    </div>
  )
}
