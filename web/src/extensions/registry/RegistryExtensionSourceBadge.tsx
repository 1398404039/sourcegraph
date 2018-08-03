import CircleRemoveAlternateIcon from '@sourcegraph/icons/lib/CircleRemoveAlternate'
import GlobeIcon from '@sourcegraph/icons/lib/Globe'
import * as React from 'react'
import * as GQL from '../../backend/graphqlschema'
import { LinkOrSpan } from '../../components/LinkOrSpan'

export const RegistryExtensionSourceBadge: React.SFC<{
    extension: Pick<GQL.IRegistryExtension, 'remoteURL' | 'registryName' | 'isLocal'>
    showIcon?: boolean
    showText?: boolean
    showRegistryName?: boolean
    className?: string
}> = ({ extension, showIcon, showText, showRegistryName, className = '' }) => (
    <LinkOrSpan
        to={extension.remoteURL}
        target="_blank"
        className={`text-muted text-nowrap d-inline-flex align-items-center ${className}`}
        data-tooltip={
            extension.isLocal
                ? 'Published on this site'
                : `Published on external extension registry ${extension.registryName}`
        }
    >
        {showIcon &&
            (extension.isLocal ? (
                <CircleRemoveAlternateIcon className="icon-inline mr-1" />
            ) : (
                <GlobeIcon className="icon-inline mr-1" />
            ))}
        {showText && (extension.isLocal ? 'Local' : showRegistryName ? extension.registryName : 'External')}
    </LinkOrSpan>
)
