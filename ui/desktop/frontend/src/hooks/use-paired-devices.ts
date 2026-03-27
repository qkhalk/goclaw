import { useState, useEffect, useCallback } from 'react'
import { getWsClient } from '../lib/ws'
import type { PendingPairing, PairedDevice } from '../types/channel'

export function usePairedDevices() {
  const [pendingPairings, setPendingPairings] = useState<PendingPairing[]>([])
  const [pairedDevices, setPairedDevices] = useState<PairedDevice[]>([])
  const [loading, setLoading] = useState(true)

  const fetchDevices = useCallback(async () => {
    try {
      const ws = getWsClient()
      const raw = await ws.call('device.pair.list')
      console.log('[paired-devices] raw response:', raw)
      const res = raw as { pending: PendingPairing[]; paired: PairedDevice[] }
      console.log('[paired-devices] pending:', res.pending?.length, 'paired:', res.paired?.length)
      setPendingPairings(res.pending ?? [])
      setPairedDevices(res.paired ?? [])
    } catch (err) {
      console.error('[paired-devices] error:', err)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchDevices()
    let unsub1: (() => void) | undefined
    let unsub2: (() => void) | undefined
    try {
      const ws = getWsClient()
      unsub1 = ws.on('device.pair.requested', () => { fetchDevices() })
      unsub2 = ws.on('device.pair.resolved', () => { fetchDevices() })
    } catch { /* ws not ready */ }
    return () => { unsub1?.(); unsub2?.() }
  }, [fetchDevices])

  const approvePairing = useCallback(async (code: string) => {
    await getWsClient().call('device.pair.approve', { code, approvedBy: 'desktop' })
    fetchDevices()
  }, [fetchDevices])

  const denyPairing = useCallback(async (code: string) => {
    await getWsClient().call('device.pair.deny', { code })
    fetchDevices()
  }, [fetchDevices])

  const revokePairing = useCallback(async (senderId: string, channel: string) => {
    await getWsClient().call('device.pair.revoke', { senderId, channel })
    fetchDevices()
  }, [fetchDevices])

  return { pendingPairings, pairedDevices, loading, refresh: fetchDevices, approvePairing, denyPairing, revokePairing }
}
