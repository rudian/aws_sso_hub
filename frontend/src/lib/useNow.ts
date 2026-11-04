import { useEffect, useState } from 'react'
import { now as clockNow } from './clock'

/** Current time in ms, refreshed every `intervalMs`. Honours the mock clock (?now=) in mock mode. */
export function useNow(intervalMs: number): number {
  const [now, setNow] = useState(() => clockNow())
  useEffect(() => {
    setNow(clockNow())
    const id = window.setInterval(() => setNow(clockNow()), intervalMs)
    return () => window.clearInterval(id)
  }, [intervalMs])
  return now
}
