import * as React from 'react'
import { Link } from 'react-router-dom'
import { eventLogger } from '../tracking/eventLogger'
import { toBlobURL, toPrettyRepoURL, toTreeURL } from '../util/url'

export interface Props {
    path: string
    partToUrl: (i: number) => string | undefined
    partToClassName?: (i: number) => string
}

export class Breadcrumb extends React.Component<Props, {}> {
    public render(): JSX.Element | null {
        const parts = this.props.path.split('/')
        const spans: JSX.Element[] = []
        for (const [i, part] of parts.entries()) {
            const link = this.props.partToUrl(i)
            const className = `part ${this.props.partToClassName ? this.props.partToClassName(i) : ''} ${
                i === parts.length - 1 ? 'part-last' : ''
            }`
            if (link) {
                spans.push(
                    <Link onClick={this.logBreadcrumbClicked} key={i} className={className} to={link} title={part}>
                        {part}
                    </Link>
                )
            } else {
                spans.push(
                    <span key={i} className={className} title={part}>
                        {part}
                    </span>
                )
            }
            if (i < parts.length - 1) {
                spans.push(
                    <span key={'sep' + i} className="breadcrumb__separator">
                        /
                    </span>
                )
            }
        }
        return (
            // Important: do not put spaces between the breadcrumbs or spaces will get added when copying the path
            <span className="breadcrumb">{spans}</span>
        )
    }

    private logBreadcrumbClicked = () => eventLogger.log('SearchResultsBreadcrumbClicked')
}

export interface RepoBreadcrumbProps {
    repoPath: string
    rev?: string
    filePath?: string
    disableLinks?: boolean
    isDirectory?: boolean
}

/**
 *  Returns the friendly display form of the repository path (e.g., removing "github.com/").
 */
export function displayRepoPath(repoPath: string): string {
    let parts = repoPath.split('/')
    if (parts.length >= 3 && parts[0].includes('.')) {
        parts = parts.slice(1) // remove hostname from repo path (reduce visual noise)
    }
    return parts.join('/')
}

export class RepoBreadcrumb extends React.Component<RepoBreadcrumbProps, {}> {
    public render(): JSX.Element | null {
        return (
            <Breadcrumb
                path={displayRepoPath(this.props.repoPath) + (this.props.filePath ? '/' + this.props.filePath : '')}
                partToUrl={this.partToUrl}
                partToClassName={this.partToClassName}
            />
        )
    }

    private partToUrl = (i: number): string | undefined => {
        if (this.props.disableLinks) {
            return undefined
        }
        const trimmedUri = this.props.repoPath
            .split('/')
            .slice(1)
            .join('/') // remove first path part
        const uriParts = trimmedUri.split('/')
        if (i < uriParts.length - 1) {
            return undefined
        }
        if (i === uriParts.length - 1) {
            return toPrettyRepoURL(this.props)
        }
        if (this.props.filePath) {
            const j = i - uriParts.length
            const pathParts = this.props.filePath.split('/')
            if (this.props.isDirectory || j < pathParts.length - 1) {
                return toTreeURL({
                    repoPath: this.props.repoPath,
                    rev: this.props.rev,
                    filePath: pathParts.slice(0, j + 1).join('/'),
                })
            }
            return toBlobURL({ repoPath: this.props.repoPath, rev: this.props.rev, filePath: this.props.filePath })
        }
        return undefined
    }

    private partToClassName = (i: number) => {
        const trimmedUri = this.props.repoPath
            .split('/')
            .slice(1)
            .join('/') // remove first path part
        const uriParts = trimmedUri.split('/')
        if (i < uriParts.length) {
            return 'part-repo'
        }
        if (this.props.filePath) {
            const j = i - uriParts.length
            const pathParts = this.props.filePath.split('/')
            if (j < pathParts.length - 1) {
                return 'part-directory'
            }
            return 'part-file'
        }
        return ''
    }
}
