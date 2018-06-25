import * as React from 'react'
import { RouteComponentProps } from 'react-router'
import { Observable, Subject, Subscription } from 'rxjs'
import { distinctUntilChanged, map, startWith } from 'rxjs/operators'
import { ExtensionsProps } from '../../backend/features'
import { gql, queryGraphQL } from '../../backend/graphql'
import * as GQL from '../../backend/graphqlschema'
import { FilteredConnection } from '../../components/FilteredConnection'
import { createAggregateError } from '../../util/errors'
import { FileDiffNode, FileDiffNodeProps } from './FileDiffNode'
import { RepositoryCompareAreaPageProps } from './RepositoryCompareArea'

export function queryRepositoryComparisonFileDiffs(args: {
    repo: GQL.ID
    base: string | null
    head: string | null
    first?: number
}): Observable<GQL.IFileDiffConnection> {
    return queryGraphQL(
        gql`
            query RepositoryComparisonDiff($repo: ID!, $base: String, $head: String, $first: Int) {
                node(id: $repo) {
                    ... on Repository {
                        comparison(base: $base, head: $head) {
                            fileDiffs(first: $first) {
                                nodes {
                                    ...FileDiffFields
                                }
                                totalCount
                                pageInfo {
                                    hasNextPage
                                }
                                diffStat {
                                    ...DiffStatFields
                                }
                            }
                        }
                    }
                }
            }

            fragment FileDiffFields on FileDiff {
                oldPath
                newPath
                mostRelevantFile {
                    url
                }
                hunks {
                    oldRange {
                        ...FileDiffHunkRangeFields
                    }
                    oldNoNewlineAt
                    newRange {
                        ...FileDiffHunkRangeFields
                    }
                    section
                    body
                }
                stat {
                    ...DiffStatFields
                }
                internalID
            }

            fragment FileDiffHunkRangeFields on FileDiffHunkRange {
                startLine
                lines
            }

            fragment DiffStatFields on DiffStat {
                added
                changed
                deleted
            }
        `,
        args
    ).pipe(
        map(({ data, errors }) => {
            if (!data || !data.node) {
                throw createAggregateError(errors)
            }
            const repo = data.node as GQL.IRepository
            if (!repo.comparison || !repo.comparison.fileDiffs || errors) {
                throw createAggregateError(errors)
            }
            return repo.comparison.fileDiffs
        })
    )
}

interface Props extends RepositoryCompareAreaPageProps, RouteComponentProps<{}>, ExtensionsProps {
    /** The base of the comparison. */
    base: { repoPath: string; repoID: GQL.ID; rev: string | null; commitID: string }

    /** The head of the comparison. */
    head: { repoPath: string; repoID: GQL.ID; rev: string | null; commitID: string }
}

class FilteredFileDiffConnection extends FilteredConnection<
    GQL.IFileDiff,
    Pick<FileDiffNodeProps, 'base' | 'head' | 'lineNumbers' | 'className' | 'extensions' | 'location' | 'history'>
> {}

/** A page with the file diffs in the comparison. */
export class RepositoryCompareDiffPage extends React.PureComponent<Props> {
    private componentUpdates = new Subject<Props>()
    private updates = new Subject<void>()
    private subscriptions = new Subscription()

    public componentDidMount(): void {
        this.subscriptions.add(
            this.componentUpdates
                .pipe(
                    startWith(this.props),
                    distinctUntilChanged(
                        (a, b) => a.repo.id === b.repo.id && a.base.rev === b.base.rev && a.head.rev === b.head.rev
                    )
                )
                .subscribe(() => this.updates.next())
        )
    }

    public componentWillUpdate(nextProps: Props): void {
        this.componentUpdates.next(nextProps)
    }

    public componentWillUnmount(): void {
        this.subscriptions.unsubscribe()
    }

    public render(): JSX.Element | null {
        return (
            <div className="repository-compare-page">
                <FilteredFileDiffConnection
                    listClassName="list-group list-group-flush"
                    noun="changed file"
                    pluralNoun="changed files"
                    queryConnection={this.queryDiffs}
                    nodeComponent={FileDiffNode}
                    nodeComponentProps={{
                        base: { ...this.props.base, rev: this.props.base.rev || 'HEAD' },
                        head: { ...this.props.head, rev: this.props.head.rev || 'HEAD' },
                        lineNumbers: true,
                        extensions: this.props.extensions,
                        location: this.props.location,
                        history: this.props.history,
                    }}
                    defaultFirst={25}
                    hideSearch={true}
                    noSummaryIfAllNodesVisible={true}
                    history={this.props.history}
                    location={this.props.location}
                />
            </div>
        )
    }

    private queryDiffs = (args: { first?: number }): Observable<GQL.IFileDiffConnection> =>
        queryRepositoryComparisonFileDiffs({
            ...args,
            repo: this.props.repo.id,
            base: this.props.base.commitID,
            head: this.props.head.commitID,
        })
}
