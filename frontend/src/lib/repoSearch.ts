// The repository search ranking (design 5.2 "Find a repository by typing"). One pure module: the Projects search uses
// it now and the Cmd/Ctrl+O repository switcher (A2-16) reuses it, so both rank the same way.
//
// Ranking, best first:
//   0  the local folder name starts with the text
//   1  a word inside the folder name starts with it (after `_`, `-`, `.` or a case change: `agent` finds `acme_agent_portal`)
//   2  the text is anywhere in the folder name
//   3  the remote name contains it
//   4  the CodeCommit name contains it
//   5  the path contains it

export interface Searchable {
  localName: string
  remoteName?: string
  /** The CodeCommit repository name (empty for other providers). */
  name?: string
  displayPath?: string
}

export const RANK_PREFIX = 0
export const RANK_WORD = 1
export const RANK_FOLDER = 2
export const RANK_REMOTE = 3
export const RANK_CODECOMMIT = 4
export const RANK_PATH = 5

/** The text to search for: trimmed and lower-cased. */
export function normalizeQuery(q: string): string {
  return q.trim().toLowerCase()
}

function isWordStart(s: string, i: number): boolean {
  if (i === 0) return true
  const prev = s[i - 1]
  const cur = s[i]
  if (prev === '_' || prev === '-' || prev === '.') return true
  // a case change: "acmeAgent" -> Agent starts a word
  return prev !== prev.toUpperCase() && cur !== cur.toLowerCase()
}

/** Index of the first word start in `s` where `q` (already lower-case) begins, or -1. */
function wordIndex(s: string, q: string): number {
  const lower = s.toLowerCase()
  for (let i = lower.indexOf(q); i >= 0; i = lower.indexOf(q, i + 1)) {
    if (isWordStart(s, i)) return i
  }
  return -1
}

/** The rank of a repository for the text, or null when nothing matches. An empty text ranks everything 0. */
export function rankRepo(r: Searchable, query: string): number | null {
  const q = normalizeQuery(query)
  if (!q) return RANK_PREFIX
  const folder = r.localName.toLowerCase()
  if (folder.startsWith(q)) return RANK_PREFIX
  if (wordIndex(r.localName, q) >= 0) return RANK_WORD
  if (folder.includes(q)) return RANK_FOLDER
  if (r.remoteName?.toLowerCase().includes(q)) return RANK_REMOTE
  if (r.name?.toLowerCase().includes(q)) return RANK_CODECOMMIT
  if (r.displayPath?.toLowerCase().includes(q)) return RANK_PATH
  return null
}

/** The repositories that match, best first; repositories of the same rank keep their order. */
export function searchRepos<T extends Searchable>(repos: readonly T[], query: string): T[] {
  const ranked: { r: T; rank: number; i: number }[] = []
  repos.forEach((r, i) => {
    const rank = rankRepo(r, query)
    if (rank !== null) ranked.push({ r, rank, i })
  })
  ranked.sort((a, b) => a.rank - b.rank || a.i - b.i)
  return ranked.map((x) => x.r)
}

/**
 * The letters of `text` to draw bold for the query: the start, else the first word start, else the first occurrence.
 * `[start, end)` or null.
 */
export function matchRange(text: string, query: string): [number, number] | null {
  const q = normalizeQuery(query)
  if (!q || !text) return null
  const lower = text.toLowerCase()
  let at = lower.startsWith(q) ? 0 : wordIndex(text, q)
  if (at < 0) at = lower.indexOf(q)
  return at < 0 ? null : [at, at + q.length]
}
