export function bytesToB64(u8: Uint8Array): string {
  let s = ''
  for (let i = 0; i < u8.length; i += 0x8000) s += String.fromCharCode.apply(null, u8.subarray(i, i + 0x8000) as unknown as number[])
  return btoa(s)
}

export function b64ToBytes(s: string): Uint8Array {
  const bin = atob(s)
  const u = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) u[i] = bin.charCodeAt(i)
  return u
}

/** What xterm's onData gives (a JS string) as UTF-8 bytes, base64. */
export function textToB64(s: string): string {
  return bytesToB64(new TextEncoder().encode(s))
}

/** What xterm's onBinary gives: one char per byte (latin1), base64. */
export function binaryStringToB64(s: string): string {
  const u = new Uint8Array(s.length)
  for (let i = 0; i < s.length; i++) u[i] = s.charCodeAt(i) & 255
  return bytesToB64(u)
}
