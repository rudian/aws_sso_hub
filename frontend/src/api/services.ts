import * as AccountReal from '../../bindings/github.com/rudian/aws_sso_hub/internal/app/accountservice'
import * as MigrationReal from '../../bindings/github.com/rudian/aws_sso_hub/internal/app/migrationservice'
import * as RepoReal from '../../bindings/github.com/rudian/aws_sso_hub/internal/app/reposervice'
import * as GitGlobalReal from '../../bindings/github.com/rudian/aws_sso_hub/internal/app/gitglobalservice'
import * as SystemReal from '../../bindings/github.com/rudian/aws_sso_hub/internal/app/systemservice'
import { useMocks } from './env'
import { mock } from './mock'

// The facade: pages import these objects from 'src/api'. Mocks are typed against the generated modules, so tsc fails
// when a Go method changes and the mock does not.
export const Accounts: typeof AccountReal = useMocks ? mock.accounts : AccountReal
export const Migration: typeof MigrationReal = useMocks ? mock.migration : MigrationReal
export const Repos: typeof RepoReal = useMocks ? mock.repos : RepoReal
export const GitGlobal: typeof GitGlobalReal = useMocks ? mock.gitGlobal : GitGlobalReal
export const System: typeof SystemReal = useMocks ? mock.system : SystemReal
