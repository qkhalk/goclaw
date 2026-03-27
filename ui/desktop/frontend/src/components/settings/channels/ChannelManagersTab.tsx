import { useState, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { Combobox } from '../../common/Combobox'
import type { GroupManagerGroupInfo, GroupManagerData, ChannelContact } from '../../../types/channel'

interface ChannelManagersTabProps {
  listManagerGroups: () => Promise<GroupManagerGroupInfo[]>
  listManagers: (groupId: string) => Promise<GroupManagerData[]>
  addManager: (groupId: string, userId: string, displayName?: string, username?: string) => Promise<void>
  removeManager: (groupId: string, userId: string) => Promise<void>
  listContacts: (search: string) => Promise<ChannelContact[]>
}

function shortGroupId(id: string): string {
  return id.match(/^group:[^:]+:(.+)$/)?.[1] ?? id
}

export function ChannelManagersTab({
  listManagerGroups, listManagers, addManager, removeManager, listContacts,
}: ChannelManagersTabProps) {
  const { t } = useTranslation('channels')
  const [groups, setGroups] = useState<GroupManagerGroupInfo[]>([])
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})
  const [managersMap, setManagersMap] = useState<Record<string, GroupManagerData[]>>({})
  const [loadingMap, setLoadingMap] = useState<Record<string, boolean>>({})
  const [contactOptions, setContactOptions] = useState<{ value: string; label: string }[]>([])
  // Per-group inline add: userId
  const [inlineUserId, setInlineUserId] = useState<Record<string, string>>({})
  // Standalone add form
  const [newGroupId, setNewGroupId] = useState('')
  const [newUserId, setNewUserId] = useState('')
  const [addingMap, setAddingMap] = useState<Record<string, boolean>>({})
  const [error, setError] = useState('')

  const loadGroups = useCallback(async () => {
    try {
      const data = await listManagerGroups()
      setGroups(data)
    } catch {
      setGroups([])
    }
  }, [listManagerGroups])

  useEffect(() => { loadGroups() }, [loadGroups])

  const handleToggle = async (groupId: string) => {
    const next = !expanded[groupId]
    setExpanded((prev) => ({ ...prev, [groupId]: next }))
    if (next && !managersMap[groupId]) {
      setLoadingMap((prev) => ({ ...prev, [groupId]: true }))
      try {
        const data = await listManagers(groupId)
        setManagersMap((prev) => ({ ...prev, [groupId]: data }))
      } finally {
        setLoadingMap((prev) => ({ ...prev, [groupId]: false }))
      }
    }
  }

  const handleContactSearch = useCallback(async (search: string) => {
    try {
      const contacts = await listContacts(search)
      setContactOptions(contacts.map((c) => ({
        value: c.sender_id,
        label: c.display_name ? `${c.display_name} (${c.sender_id})` : c.sender_id,
      })))
    } catch {
      setContactOptions([])
    }
  }, [listContacts])

  const handleInlineAdd = async (groupId: string) => {
    const userId = inlineUserId[groupId]?.trim()
    if (!userId) return
    setAddingMap((prev) => ({ ...prev, [groupId]: true }))
    setError('')
    try {
      await addManager(groupId, userId)
      setInlineUserId((prev) => ({ ...prev, [groupId]: '' }))
      const data = await listManagers(groupId)
      setManagersMap((prev) => ({ ...prev, [groupId]: data }))
      await loadGroups()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('managers.addFailed'))
    } finally {
      setAddingMap((prev) => ({ ...prev, [groupId]: false }))
    }
  }

  const handleRemove = async (groupId: string, userId: string) => {
    setError('')
    try {
      await removeManager(groupId, userId)
      setManagersMap((prev) => ({
        ...prev,
        [groupId]: (prev[groupId] ?? []).filter((m) => m.user_id !== userId),
      }))
      await loadGroups()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('managers.removeFailed'))
    }
  }

  const handleStandaloneAdd = async () => {
    const gid = newGroupId.trim()
    const uid = newUserId.trim()
    if (!gid || !uid) return
    setAddingMap((prev) => ({ ...prev, _new: true }))
    setError('')
    try {
      await addManager(gid, uid)
      setNewGroupId('')
      setNewUserId('')
      await loadGroups()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('managers.addFailed'))
    } finally {
      setAddingMap((prev) => ({ ...prev, _new: false }))
    }
  }

  const inputClass = 'bg-surface-tertiary border border-border rounded-lg px-2.5 py-1.5 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:ring-1 focus:ring-accent'

  return (
    <div className="space-y-4">
      {error && <p className="text-xs text-error">{error}</p>}

      {groups.length === 0 ? (
        <p className="text-sm text-text-muted py-6 text-center">{t('managers.empty')}</p>
      ) : (
        <div className="space-y-2">
          {groups.map((g) => (
            <div key={g.group_id} className="border border-border rounded-lg overflow-hidden">
              <button
                onClick={() => handleToggle(g.group_id)}
                className="w-full flex items-center justify-between px-4 py-3 bg-surface-secondary hover:bg-surface-tertiary transition-colors text-left cursor-pointer"
              >
                <span className="text-sm font-medium text-text-primary font-mono">{shortGroupId(g.group_id)}</span>
                <div className="flex items-center gap-3">
                  <span className="text-xs text-text-muted">{g.writer_count} {t('managers.writers')}</span>
                  <svg className={`w-4 h-4 text-text-muted transition-transform ${expanded[g.group_id] ? 'rotate-180' : ''}`} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}><path d="m6 9 6 6 6-6" /></svg>
                </div>
              </button>

              {expanded[g.group_id] && (
                <div className="px-4 pb-3 pt-2 bg-surface-primary space-y-3">
                  {loadingMap[g.group_id] ? (
                    <p className="text-xs text-text-muted py-2">{t('common.loading')}</p>
                  ) : (managersMap[g.group_id] ?? []).length === 0 ? (
                    <p className="text-xs text-text-muted py-2">{t('managers.noManagers')}</p>
                  ) : (
                    <div className="divide-y divide-border">
                      {(managersMap[g.group_id] ?? []).map((m) => (
                        <div key={m.user_id} className="flex items-center justify-between py-2">
                          <div>
                            <span className="text-xs font-mono text-text-primary">{m.user_id}</span>
                            {m.display_name && <span className="text-xs text-text-muted ml-2">{m.display_name}</span>}
                            {m.username && <span className="text-xs text-text-muted ml-1">@{m.username}</span>}
                          </div>
                          <button onClick={() => handleRemove(g.group_id, m.user_id)} className="text-xs text-error hover:opacity-80 cursor-pointer transition-opacity">
                            {t('managers.remove')}
                          </button>
                        </div>
                      ))}
                    </div>
                  )}
                  <div className="flex gap-2 pt-1">
                    <div className="flex-1">
                      <Combobox
                        value={inlineUserId[g.group_id] ?? ''}
                        onChange={(v) => { setInlineUserId((prev) => ({ ...prev, [g.group_id]: v })); handleContactSearch(v) }}
                        options={contactOptions}
                        placeholder={t('managers.userIdPlaceholder')}
                      />
                    </div>
                    <button
                      onClick={() => handleInlineAdd(g.group_id)}
                      disabled={addingMap[g.group_id] || !inlineUserId[g.group_id]?.trim()}
                      className="px-3 py-1.5 bg-accent text-white text-xs rounded-lg disabled:opacity-50 cursor-pointer hover:opacity-90 transition-opacity"
                    >
                      {t('managers.add')}
                    </button>
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      {/* Standalone add form for new groups */}
      <div className="border border-border rounded-lg p-4 space-y-3">
        <p className="text-xs font-medium text-text-secondary">{t('managers.addToGroup')}</p>
        <input
          value={newGroupId}
          onChange={(e) => setNewGroupId(e.target.value)}
          placeholder={t('managers.groupIdPlaceholder')}
          className={`w-full ${inputClass}`}
        />
        <div className="flex gap-2">
          <div className="flex-1">
            <Combobox
              value={newUserId}
              onChange={(v) => { setNewUserId(v); handleContactSearch(v) }}
              options={contactOptions}
              placeholder={t('managers.userIdPlaceholder')}
            />
          </div>
          <button
            onClick={handleStandaloneAdd}
            disabled={addingMap['_new'] || !newGroupId.trim() || !newUserId.trim()}
            className="px-3 py-1.5 bg-accent text-white text-xs rounded-lg disabled:opacity-50 cursor-pointer hover:opacity-90 transition-opacity"
          >
            {t('managers.add')}
          </button>
        </div>
      </div>
    </div>
  )
}
