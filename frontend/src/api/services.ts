import * as AccountReal from '../../bindings/github.com/rudian/codepass/internal/app/accountservice'
import * as MigrationReal from '../../bindings/github.com/rudian/codepass/internal/app/migrationservice'
import * as RepoReal from '../../bindings/github.com/rudian/codepass/internal/app/reposervice'
import * as GitGlobalReal from '../../bindings/github.com/rudian/codepass/internal/app/gitglobalservice'
import * as SystemReal from '../../bindings/github.com/rudian/codepass/internal/app/systemservice'
import * as ToolReal from '../../bindings/github.com/rudian/codepass/internal/app/toolservice'
import * as IdentityReal from '../../bindings/github.com/rudian/codepass/internal/app/identityservice'
import * as UpdateReal from '../../bindings/github.com/rudian/codepass/internal/app/updateservice'
import * as NotifyReal from '../../bindings/github.com/rudian/codepass/internal/app/notifyservice'
import { useMocks } from './env'
import { mock } from './mock'

// The facade: pages import these objects from 'src/api'. Mocks are typed against the generated modules, so tsc fails
// when a Go method changes and the mock does not.
export const Accounts: typeof AccountReal = useMocks ? mock.accounts : AccountReal
export const Migration: typeof MigrationReal = useMocks ? mock.migration : MigrationReal
export const Repos: typeof RepoReal = useMocks ? mock.repos : RepoReal
export const GitGlobal: typeof GitGlobalReal = useMocks ? mock.gitGlobal : GitGlobalReal
export const System: typeof SystemReal = useMocks ? mock.system : SystemReal
export const Tools: typeof ToolReal = useMocks ? mock.tools : ToolReal
export const Identity: typeof IdentityReal = useMocks ? mock.identity : IdentityReal
export const Updates: typeof UpdateReal = useMocks ? mock.update : UpdateReal
export const Notify: typeof NotifyReal = useMocks ? mock.notify : NotifyReal
