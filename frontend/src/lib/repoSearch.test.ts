import { describe, expect, it } from 'vitest'
import { RANK_CODECOMMIT, RANK_FOLDER, RANK_PATH, RANK_PREFIX, RANK_REMOTE, RANK_WORD, matchRange, rankRepo, searchRepos } from './repoSearch'

const repo = (localName: string, extra: { remoteName?: string; name?: string; displayPath?: string } = {}) => ({ localName, ...extra })

describe('rankRepo', () => {
  it('ranks a folder name that starts with the text first', () => {
    expect(rankRepo(repo('acme_api'), 'acm')).toBe(RANK_PREFIX)
    expect(rankRepo(repo('Acme_api'), ' ACM ')).toBe(RANK_PREFIX)
  })
  it('finds a word after _, -, . or a case change', () => {
    expect(rankRepo(repo('acme_agent_portal'), 'agent')).toBe(RANK_WORD)
    expect(rankRepo(repo('acme-agent'), 'agent')).toBe(RANK_WORD)
    expect(rankRepo(repo('acme.agent'), 'agent')).toBe(RANK_WORD)
    expect(rankRepo(repo('acmeAgentPortal'), 'agent')).toBe(RANK_WORD)
    expect(rankRepo(repo('acmeAgentPortal'), 'portal')).toBe(RANK_WORD)
  })
  it('a match in the middle of a word is "anywhere in the folder name"', () => {
    expect(rankRepo(repo('acme_reagent'), 'agent')).toBe(RANK_FOLDER)
    expect(rankRepo(repo('acmeagent'), 'agent')).toBe(RANK_FOLDER)
  })
  it('a later word start wins over an earlier mid-word hit', () => {
    expect(rankRepo(repo('reagent_agent'), 'agent')).toBe(RANK_WORD)
  })
  it('falls back to the remote name, the CodeCommit name and the path', () => {
    expect(rankRepo(repo('api-main', { remoteName: 'acme_api' }), 'acme')).toBe(RANK_REMOTE)
    expect(rankRepo(repo('api-main', { name: 'billing-service' }), 'billing')).toBe(RANK_CODECOMMIT)
    expect(rankRepo(repo('api-main', { displayPath: '~/work/globex/api-main' }), 'globex')).toBe(RANK_PATH)
  })
  it('returns null without a match and 0 for an empty text', () => {
    expect(rankRepo(repo('acme_api', { remoteName: 'x', displayPath: '~/a' }), 'zzz')).toBeNull()
    expect(rankRepo(repo('acme_api'), '   ')).toBe(RANK_PREFIX)
  })
})

describe('searchRepos', () => {
  it('orders by rank and keeps the order inside a rank', () => {
    const list = [
      repo('zeta_agent'),
      repo('x', { displayPath: '~/agent/x' }),
      repo('agent_two'),
      repo('reagentless'),
      repo('agent_one'),
      repo('unrelated'),
    ]
    expect(searchRepos(list, 'agent').map((r) => r.localName)).toEqual(['agent_two', 'agent_one', 'zeta_agent', 'reagentless', 'x'])
  })
})

describe('matchRange', () => {
  it('marks the start, else the first word, else the first hit', () => {
    expect(matchRange('acme_agent_portal', 'acm')).toEqual([0, 3])
    expect(matchRange('acme_agent_portal', 'AGENT')).toEqual([5, 10])
    expect(matchRange('acme_reagent', 'agent')).toEqual([7, 12])
    expect(matchRange('acme_api', 'zzz')).toBeNull()
    expect(matchRange('acme_api', '')).toBeNull()
  })
})
