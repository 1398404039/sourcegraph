import CircleChevronLeft from '@sourcegraph/icons/lib/CircleChevronLeft'
import * as H from 'history'
import * as React from 'react'
import { Link } from 'react-router-dom'
import { Observable } from 'rxjs/Observable'
import { map } from 'rxjs/operators/map'
import { replaceRevisionInURL } from '.'
import { gql, queryGraphQL } from '../backend/graphql'
import * as GQL from '../backend/graphqlschema'
import { FilteredConnection, FilteredConnectionQueryArgs } from '../components/FilteredConnection'
import { TabsWithLocalStorageViewStatePersistence } from '../components/Tabs'
import { eventLogger } from '../tracking/eventLogger'
import { createAggregateError } from '../util/errors'
import { memoizeObservable } from '../util/memoize'

const fetchGitRefs = memoizeObservable(
    (args: {
        repo: GQL.ID
        first?: number
        query?: string
        type?: GQL.GitRefType
    }): Observable<GQL.IGitRefConnection> =>
        queryGraphQL(
            gql`
                query RepositoryGitRefs($repo: ID!, $first: Int, $query: String, $type: GitRefType) {
                    node(id: $repo) {
                        ... on Repository {
                            gitRefs(first: $first, query: $query, type: $type) {
                                nodes {
                                    id
                                    name
                                    displayName
                                    abbrevName
                                    type
                                }
                                totalCount
                            }
                        }
                    }
                }
            `,
            args
        ).pipe(
            map(({ data, errors }) => {
                if (!data || !data.node || !(data.node as GQL.IRepository).gitRefs) {
                    throw createAggregateError(errors)
                }
                return (data.node as GQL.IRepository).gitRefs
            })
        ),
    x => JSON.stringify(x)
)

const fetchRepositoryCommits = memoizeObservable(
    (args: { repo: GQL.ID; rev?: string; first?: number; query?: string }): Observable<GQL.IGitCommitConnection> =>
        queryGraphQL(
            gql`
                query RepositoryGitCommit($repo: ID!, $first: Int, $rev: String!, $query: String) {
                    node(id: $repo) {
                        ... on Repository {
                            commit(rev: $rev) {
                                ancestors(first: $first, query: $query) {
                                    nodes {
                                        id
                                        oid
                                        abbreviatedOID
                                        author {
                                            person {
                                                name
                                                avatarURL
                                            }
                                            date
                                        }
                                        subject
                                    }
                                    pageInfo {
                                        hasNextPage
                                    }
                                }
                            }
                        }
                    }
                }
            `,
            args
        ).pipe(
            map(({ data, errors }) => {
                if (
                    !data ||
                    !data.node ||
                    !(data.node as GQL.IRepository).commit ||
                    !(data.node as GQL.IRepository).commit!.ancestors
                ) {
                    throw createAggregateError(errors)
                }
                return (data.node as GQL.IRepository).commit!.ancestors
            })
        ),
    x => JSON.stringify(x)
)

interface GitRefNodeProps {
    node: GQL.IGitRef

    defaultBranch: string | undefined
    currentRev: string | undefined

    location: H.Location
}

export const GitRefNode: React.SFC<GitRefNodeProps> = ({ node, defaultBranch, currentRev, location }) => {
    let isCurrent: boolean
    if (currentRev) {
        isCurrent = node.name === currentRev || node.abbrevName === currentRev
    } else {
        isCurrent = node.name === `refs/heads/${defaultBranch}`
    }

    return (
        <li key={node.id} className="popover__node">
            <Link
                to={replaceRevisionInURL(location.pathname + location.search + location.hash, node.abbrevName)}
                className={`popover__node-link ${isCurrent ? 'popover__node-link--active' : ''}`}
            >
                {node.displayName}
                {isCurrent && (
                    <CircleChevronLeft className="icon-inline popover__node-link-icon" data-tooltip="Current" />
                )}
            </Link>
        </li>
    )
}

interface GitCommitNodeProps {
    node: GQL.IGitCommit

    currentCommitID: string | undefined

    location: H.Location
}

export const GitCommitNode: React.SFC<GitCommitNodeProps> = ({ node, currentCommitID, location }) => {
    const isCurrent = currentCommitID === (node.oid as string)
    return (
        <li key={node.oid} className="popover__node revisions-popover-git-commit-node">
            <Link
                to={replaceRevisionInURL(location.pathname + location.search + location.hash, node.oid as string)}
                className={`popover__node-link ${
                    isCurrent ? 'popover__node-link--active' : ''
                } revisions-popover-git-commit-node__link`}
            >
                <code className="revisions-popover-git-commit-node__oid" title={node.oid}>
                    {node.abbreviatedOID}
                </code>
                <span className="revisions-popover-git-commit-node__message">{(node.subject || '').slice(0, 200)}</span>
                {isCurrent && (
                    <CircleChevronLeft
                        className="icon-inline popover__node-link-icon revisions-popover-git-commit-node__icon"
                        data-tooltip="Current commit"
                    />
                )}
            </Link>
        </li>
    )
}

interface Props {
    repo: GQL.ID
    repoPath: string
    defaultBranch: string | undefined

    /** The current revision, or undefined for the default branch. */
    currentRev: string | undefined

    currentCommitID?: string

    history: H.History
    location: H.Location
}

type RevisionsPopoverTabID = 'branches' | 'tags' | 'commits'

interface RevisionsPopoverTab {
    id: RevisionsPopoverTabID
    label: string
    noun: string
    pluralNoun: string
    type?: GQL.GitRefType
}

class FilteredGitRefConnection extends FilteredConnection<
    GQL.IGitRef,
    Pick<GitRefNodeProps, 'defaultBranch' | 'currentRev' | 'location'>
> {}

class FilteredGitCommitConnection extends FilteredConnection<
    GQL.IGitCommit,
    Pick<GitCommitNodeProps, 'currentCommitID' | 'location'>
> {}

/**
 * A popover that displays a searchable list of revisions (grouped by type) for
 * the current repository.
 */
export class RevisionsPopover extends React.PureComponent<Props> {
    private static LAST_TAB_STORAGE_KEY = 'RevisionsPopover.lastTab'

    private static TABS: RevisionsPopoverTab[] = [
        { id: 'branches', label: 'Branches', noun: 'branch', pluralNoun: 'branches', type: GQL.GitRefType.GIT_BRANCH },
        { id: 'tags', label: 'Tags', noun: 'tag', pluralNoun: 'tags', type: GQL.GitRefType.GIT_TAG },
        { id: 'commits', label: 'Commits', noun: 'commit', pluralNoun: 'commits' },
    ]

    public componentDidMount(): void {
        eventLogger.logViewEvent('RevisionsPopover')
    }

    public render(): JSX.Element | null {
        return (
            <div className="revisions-popover popover">
                <TabsWithLocalStorageViewStatePersistence
                    tabs={RevisionsPopover.TABS}
                    storageKey={RevisionsPopover.LAST_TAB_STORAGE_KEY}
                    className="revisions-popover__tabs"
                >
                    {RevisionsPopover.TABS.map(
                        (tab, i) =>
                            tab.type ? (
                                <FilteredGitRefConnection
                                    key={tab.id}
                                    className="popover__content"
                                    showMoreClassName="popover__show-more"
                                    compact={true}
                                    noun={tab.noun}
                                    pluralNoun={tab.pluralNoun}
                                    queryConnection={
                                        tab.type === 'GIT_BRANCH' ? this.queryGitBranches : this.queryGitTags
                                    }
                                    nodeComponent={GitRefNode}
                                    nodeComponentProps={
                                        {
                                            defaultBranch: this.props.defaultBranch,
                                            currentRev: this.props.currentRev,
                                            location: this.props.location,
                                        } as Pick<GitRefNodeProps, 'defaultBranch' | 'currentRev' | 'location'>
                                    }
                                    defaultFirst={50}
                                    autoFocus={true}
                                    history={this.props.history}
                                    location={this.props.location}
                                    noSummaryIfAllNodesVisible={true}
                                    shouldUpdateURLQuery={false}
                                />
                            ) : (
                                <FilteredGitCommitConnection
                                    key={tab.id}
                                    className="popover__content"
                                    compact={true}
                                    noun={tab.noun}
                                    pluralNoun={tab.pluralNoun}
                                    queryConnection={this.queryRepositoryCommits}
                                    nodeComponent={GitCommitNode}
                                    nodeComponentProps={
                                        {
                                            currentCommitID: this.props.currentCommitID,
                                            location: this.props.location,
                                        } as Pick<GitCommitNodeProps, 'currentCommitID' | 'location'>
                                    }
                                    defaultFirst={15}
                                    autoFocus={true}
                                    history={this.props.history}
                                    location={this.props.location}
                                    noSummaryIfAllNodesVisible={true}
                                    shouldUpdateURLQuery={false}
                                />
                            )
                    )}
                </TabsWithLocalStorageViewStatePersistence>
            </div>
        )
    }

    private queryGitBranches = (args: FilteredConnectionQueryArgs): Observable<GQL.IGitRefConnection> =>
        fetchGitRefs({ ...args, repo: this.props.repo, type: GQL.GitRefType.GIT_BRANCH })

    private queryGitTags = (args: FilteredConnectionQueryArgs): Observable<GQL.IGitRefConnection> =>
        fetchGitRefs({ ...args, repo: this.props.repo, type: GQL.GitRefType.GIT_TAG })

    private queryRepositoryCommits = (args: FilteredConnectionQueryArgs): Observable<GQL.IGitCommitConnection> =>
        fetchRepositoryCommits({
            ...args,
            repo: this.props.repo,
            rev: this.props.currentRev || this.props.defaultBranch,
        })
}
