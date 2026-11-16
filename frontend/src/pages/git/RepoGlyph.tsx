import { Folder } from 'lucide-react'
import { ProviderMark } from '../../components'
import type { ProviderKind } from '../../components/ProviderMark'

/** The provider mark of a repository, or a plain folder when the provider is not known. */
export function RepoGlyph({ provider }: { provider?: ProviderKind }) {
  if (provider) return <ProviderMark provider={provider} />
  return (
    <span className="prov" aria-hidden="true">
      <Folder size={15} />
    </span>
  )
}
