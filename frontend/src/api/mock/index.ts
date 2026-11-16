import { mockAccountService } from './accounts'
import { mockMigrationService } from './migration'
import { mockRepoService } from './repos'
import { mockGitGlobalService } from './gitglobal'
import { mockSystemService, mockToolService } from './system'
import { mockIdentityService } from './identity'

/** Typed mock implementations of the bound services (accounts, migration, repos, git global, system, tools, identity). */
export const mock = {
  accounts: mockAccountService,
  migration: mockMigrationService,
  repos: mockRepoService,
  gitGlobal: mockGitGlobalService,
  system: mockSystemService,
  tools: mockToolService,
  identity: mockIdentityService,
}
