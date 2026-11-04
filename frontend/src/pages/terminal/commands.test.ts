import { describe, expect, it } from 'vitest'
import { ProfileKind } from '../../api/models'
import { cmdValue, codecommitHost, defaultShell, hasUnsafeName, posixQuote, powershellQuote, terminalCommands } from './commands'
import type { ShellId } from './commands'

const sso = { name: 'initech-admin', kind: ProfileKind.ProfileKindSSO, portal: 'initech', region: 'us-west-2' }

describe('terminalCommands', () => {
  it('zsh/bash', () => {
    expect(terminalCommands(sso, 'posix')).toEqual({
      useHere: 'export AWS_PROFILE=initech-admin',
      signIn: 'aws sso login --sso-session initech',
      whoami: 'aws sts get-caller-identity --profile initech-admin',
      clone: 'git clone https://git-codecommit.us-west-2.amazonaws.com/v1/repos/<repo-name>',
    })
  })
  it('PowerShell and cmd', () => {
    expect(terminalCommands(sso, 'powershell').useHere).toBe("$env:AWS_PROFILE = 'initech-admin'")
    expect(terminalCommands(sso, 'cmd').useHere).toBe('set "AWS_PROFILE=initech-admin"')
  })
  it('legacy SSO signs in by profile; static keys have no sign-in', () => {
    expect(terminalCommands({ name: 'old', kind: ProfileKind.ProfileKindLegacySSO, region: 'eu-west-1' }, 'posix').signIn).toBe('aws sso login --profile old')
    expect(terminalCommands({ name: 'acme', kind: ProfileKind.ProfileKindStatic, region: 'eu-west-1' }, 'posix').signIn).toBeNull()
  })
  it('China regions use the .com.cn endpoint', () => {
    expect(codecommitHost('cn-north-1')).toBe('git-codecommit.cn-north-1.amazonaws.com.cn')
  })
  it('default shell by platform', () => {
    expect(defaultShell('windows')).toBe('powershell')
    expect(defaultShell('darwin')).toBe('posix')
  })
})

const SHELLS: ShellId[] = ['posix', 'powershell', 'cmd']
const named = (name: string, kind = ProfileKind.ProfileKindLegacySSO) => ({ name, kind, region: 'eu-west-1' })

describe('names that are not safe to paste into a shell', () => {
  const unsafe = ['acme$(touch x)', 'dev;rm -rf', "a'b", 'a\u2019b', 'a&calc', '%PATH%', 'a b', 'a`b`', 'a\nb', '', 'x'.repeat(65)]
  for (const name of unsafe) {
    for (const shell of SHELLS) {
      it(`profile ${JSON.stringify(name)} in ${shell}: every command with the name is null`, () => {
        const c = terminalCommands(named(name), shell)
        expect(c.useHere).toBeNull()
        expect(c.whoami).toBeNull()
        expect(c.signIn).toBeNull()
        expect(c.clone).toContain('<repo-name>')
        expect(hasUnsafeName(named(name))).toBe(true)
      })
    }
  }

  it('an unsafe sso-session name nulls only the sign-in command', () => {
    for (const shell of SHELLS) {
      const c = terminalCommands({ ...sso, portal: 'a&calc' }, shell)
      expect(c.signIn).toBeNull()
      expect(c.useHere).not.toBeNull()
      expect(c.whoami).not.toBeNull()
    }
    expect(hasUnsafeName({ ...sso, portal: 'a&calc' })).toBe(true)
    expect(hasUnsafeName({ ...sso, portal: 'a,b' })).toBe(true)
  })

  it('a region that does not match the pattern falls back to us-east-1', () => {
    const c = terminalCommands({ ...sso, region: 'x/../evil.example#' }, 'posix')
    expect(c.clone).toBe('git clone https://git-codecommit.us-east-1.amazonaws.com/v1/repos/<repo-name>')
    expect(terminalCommands({ ...sso, region: 'us-gov-west-1' }, 'posix').clone).toContain('us-gov-west-1')
    expect(terminalCommands({ ...sso, region: 'cn-north-1' }, 'posix').clone).toContain('amazonaws.com.cn')
  })

  it('valid names keep their bare text', () => {
    for (const name of ['acme-admin', 'globex@corp', 'initech.ops']) {
      const c = terminalCommands(named(name), 'posix')
      expect(c.useHere).toBe(`export AWS_PROFILE=${name}`)
      expect(c.whoami).toBe(`aws sts get-caller-identity --profile ${name}`)
      expect(c.signIn).toBe(`aws sso login --profile ${name}`)
      expect(hasUnsafeName(named(name))).toBe(false)
    }
    expect(terminalCommands(named('globex@corp'), 'powershell').whoami).toBe('aws sts get-caller-identity --profile globex@corp')
  })

  it('PowerShell quotes the profile as a single-quoted string', () => {
    expect(terminalCommands(named('acme'), 'powershell').useHere).toBe("$env:AWS_PROFILE = 'acme'")
  })

  it('PowerShell does not leave a comma or a leading @ bare in arguments', () => {
    expect(terminalCommands(named('a,b'), 'powershell').whoami).toBe("aws sts get-caller-identity --profile 'a,b'")
    expect(terminalCommands(named('@x'), 'powershell').whoami).toBe("aws sts get-caller-identity --profile '@x'")
  })
})

// A minimal POSIX word parser: handles unquoted text, '...' and the '\'' idiom (an escaped quote between quoted runs).
function parsePosixWord(word: string): string {
  let out = ''
  let i = 0
  while (i < word.length) {
    const c = word[i]
    if (c === "'") {
      const end = word.indexOf("'", i + 1)
      if (end < 0) throw new Error('unterminated quote')
      out += word.slice(i + 1, end)
      i = end + 1
    } else if (c === '\\') {
      out += word[i + 1]
      i += 2
    } else {
      if (/[\s$`&;|<>()*?[\]{}!"#~]/.test(c)) throw new Error(`unquoted metacharacter ${c}`)
      out += c
      i++
    }
  }
  return out
}

describe('quoting helpers', () => {
  const values = ["a'b", "'", "''", "x'; rm -rf ~; echo '", 'a b', '$(id)', '`id`', 'a\\b', 'a\nb', 'a\u2019b', 'acme-admin', 'globex@corp', '', '!history', '=ls', '==x', 'a:=ls']
  it('POSIX quoting round-trips through a single-quote parser', () => {
    for (const v of values) expect(parsePosixWord(posixQuote(v)), JSON.stringify(v)).toBe(v)
  })
  it('POSIX quoting leaves harmless text bare', () => {
    expect(posixQuote('globex@corp')).toBe('globex@corp')
    expect(posixQuote('a,b=c+d/e:f')).toBe('a,b=c+d/e:f')
    expect(posixQuote("a'b")).toBe("'a'\\''b'")
  })
  it('POSIX quoting keeps zsh from expanding a leading "=" into a command path', () => {
    expect(posixQuote('=ls')).toBe("'=ls'")
    expect(posixQuote('a:=ls')).toBe("'a:=ls'")
    expect(posixQuote('a=b')).toBe('a=b')
    const c = terminalCommands({ name: '=ls', kind: ProfileKind.ProfileKindStatic, portal: '', region: '' }, 'posix')
    expect(c.useHere).toBe("export AWS_PROFILE='=ls'")
    expect(c.whoami).toBe("aws sts get-caller-identity --profile '=ls'")
  })
  it('PowerShell doubles the ASCII quote and U+2018 to U+201B', () => {
    expect(powershellQuote("a'b")).toBe("'a''b'")
    for (const q of ['\u2018', '\u2019', '\u201A', '\u201B']) expect(powershellQuote(`a${q}b`)).toBe(`'a${q}${q}b'`)
    expect(powershellQuote('acme')).toBe("'acme'")
  })
  it('cmd accepts a value only without % ^ & | < > " ! or control characters', () => {
    for (const bad of ['a%b', 'a^b', 'a&b', 'a|b', 'a<b', 'a>b', 'a"b', 'a!b', 'a\u0000b', 'a\nb', 'a\u007fb']) expect(cmdValue(bad), JSON.stringify(bad)).toBeNull()
    expect(cmdValue('acme-admin')).toBe('acme-admin')
    expect(cmdValue('globex@corp')).toBe('globex@corp')
  })
})
