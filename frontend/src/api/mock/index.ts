import { mockAccountService } from './accounts'
import { mockMigrationService } from './migration'
import { mockRepoService } from './repos'
import { mockGitGlobalService } from './gitglobal'
import { mockSystemService } from './system'

/** Typed mock implementations of the five bound services. */
export const mock = {
  accounts: mockAccountService,
  migration: mockMigrationService,
  repos: mockRepoService,
  gitGlobal: mockGitGlobalService,
  system: mockSystemService,
}
