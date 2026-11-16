import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mock } from './index'
import { getWorld, resetWorld } from './world'

async function call<T>(p: Promise<T>, ms = 2000): Promise<T> {
  const settled = p.then(
    (v) => ({ ok: true as const, v }),
    (e: unknown) => ({ ok: false as const, e }),
  )
  await vi.advanceTimersByTimeAsync(ms)
  const r = await settled
  if (!r.ok) throw r.e
  return r.v
}

describe('FU-11c: ReattachMoved gives the old record back', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-04T04:00:00Z'))
    resetWorld('done')
  })
  afterEach(() => vi.useRealTimers())

  it('moves the row to the old ID with the old project, hidden flag, identity and test result', async () => {
    const before = await call(mock.repos.GetState())
    const moved = before.repos?.find((r) => r.movedFrom)
    expect(moved).toBeTruthy()
    const oldId = moved?.movedFrom?.oldId as string
    expect(moved?.id).not.toBe(oldId)
    expect(moved?.projectId).toBeUndefined()

    const after = await call(mock.repos.ReattachMoved(moved?.id as string))
    const row = after.repos?.find((r) => r.id === oldId)
    expect(row).toBeTruthy()
    expect(row?.localName).toBe('dashboard-main') // the folder is where it was found
    expect(row?.movedFrom).toBeUndefined()
    expect(row?.projectId).toBe('p_umbrella') // the old record's project
    expect(row?.lastTest?.repoId).toBe(oldId)
    expect(row?.lastTest?.status).toBe('ok')
    expect(after.repos?.some((r) => r.id === moved?.id)).toBe(false)
    expect(getWorld().repoIdentity[oldId]).toMatchObject({ email: 'alex@initech.example.com' })
    // The question is gone and cannot be answered twice.
    await expect(call(mock.repos.ReattachMoved(oldId))).rejects.toBeTruthy()
  })

  it('a hidden old record keeps the folder hidden', async () => {
    const w = getWorld()
    const moved = w.repos.find((r) => r.movedFrom)
    const oldId = moved?.movedFrom?.oldId as string
    w.movedRecords[oldId].hidden = true
    const after = await call(mock.repos.ReattachMoved(moved?.id as string))
    expect(after.repos?.some((r) => r.id === oldId)).toBe(false)
    expect(after.hiddenRepos?.some((r) => r.id === oldId)).toBe(true)
  })

  it('answers E_MOVED_GONE when the old record is gone or its ID is taken again', async () => {
    const w = getWorld()
    const moved = w.repos.find((r) => r.movedFrom)
    const oldId = moved?.movedFrom?.oldId as string
    const rec = w.movedRecords[oldId]
    delete w.movedRecords[oldId]
    await expect(call(mock.repos.ReattachMoved(moved?.id as string))).rejects.toMatchObject({ cause: { code: 'E_MOVED_GONE' } })
    expect(moved?.movedFrom).toBeTruthy() // nothing changed
    w.movedRecords[oldId] = rec
    const other = w.repos.find((r) => r !== moved && !r.movedFrom)
    if (!other) throw new Error('no other repository')
    const keep = other.id
    other.id = oldId
    await expect(call(mock.repos.ReattachMoved(moved?.id as string))).rejects.toMatchObject({ cause: { code: 'E_MOVED_GONE' } })
    other.id = keep
  })
})
