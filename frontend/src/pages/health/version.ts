/** Numeric dotted-version comparison ("2.38.2" < "2.39.0"). Missing parts count as 0; non-numeric parts are ignored. */
export function compareVersions(a: string, b: string): number {
  const pa = a.split('.').map((x) => parseInt(x, 10) || 0)
  const pb = b.split('.').map((x) => parseInt(x, 10) || 0)
  for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
    const d = (pa[i] ?? 0) - (pb[i] ?? 0)
    if (d !== 0) return d < 0 ? -1 : 1
  }
  return 0
}

export function versionLt(a: string, b: string): boolean {
  return compareVersions(a, b) < 0
}
