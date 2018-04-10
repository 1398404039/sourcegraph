import { ChevronRight } from '@sourcegraph/icons/lib/ChevronRight'
import LoaderIcon from '@sourcegraph/icons/lib/Loader'
import upperFirst from 'lodash/upperFirst'
import * as React from 'react'
import { Link, RouteComponentProps } from 'react-router-dom'
import { Observable } from 'rxjs/Observable'
import { merge } from 'rxjs/observable/merge'
import { of } from 'rxjs/observable/of'
import { catchError } from 'rxjs/operators/catchError'
import { distinctUntilChanged } from 'rxjs/operators/distinctUntilChanged'
import { map } from 'rxjs/operators/map'
import { switchMap } from 'rxjs/operators/switchMap'
import { Subject } from 'rxjs/Subject'
import { Subscription } from 'rxjs/Subscription'
import { gql, queryGraphQL } from '../../backend/graphql'
import { eventLogger } from '../../tracking/eventLogger'
import { createAggregateError, ErrorLike, isErrorLike } from '../../util/errors'
import { memoizeObservable } from '../../util/memoize'
import { gitBranchFragment, GitBranchNode } from './RepositoryBranchesAllPage'
import { RepositoryBranchesAreaPageProps } from './RepositoryBranchesArea'

interface Data {
    defaultBranch: GQL.IGitRef | null
    activeBranches: GQL.IGitRef[]
    hasMoreActiveBranches: boolean
}

const fetchGitBranches = memoizeObservable(
    (args: { repo: GQLID; first: number }): Observable<Data> =>
        queryGraphQL(
            gql`
                query RepositoryGitBranchesOverview($repo: ID!, $first: Int!) {
                    node(id: $repo) {
                        ... on Repository {
                            defaultBranch {
                                ...GitBranchFields
                            }
                            gitRefs(first: $first, type: GIT_BRANCH, orderBy: AUTHORED_OR_COMMITTED_AT) {
                                nodes {
                                    ...GitBranchFields
                                }
                                pageInfo {
                                    hasNextPage
                                }
                            }
                        }
                    }
                }
                ${gitBranchFragment}
            `,
            args
        ).pipe(
            map(({ data, errors }) => {
                if (!data || !data.node) {
                    throw createAggregateError(errors)
                }
                const repo = data.node as GQL.IRepository
                if (!repo.gitRefs || !repo.gitRefs.nodes) {
                    throw createAggregateError(errors)
                }
                return {
                    defaultBranch: repo.defaultBranch,
                    activeBranches: repo.gitRefs.nodes.filter(
                        // Filter out default branch from activeBranches.
                        ({ id }) => !repo.defaultBranch || repo.defaultBranch.id !== id
                    ),
                    hasMoreActiveBranches: repo.gitRefs.pageInfo.hasNextPage,
                }
            })
        ),
    args => `${args.repo}:${args.first}`
)

interface Props extends RepositoryBranchesAreaPageProps, RouteComponentProps<{}> {}

interface State {
    /** The page content, undefined while loading, or an error. */
    dataOrError?: Data | ErrorLike
}

/** A page with an overview of the repository's branches. */
export class RepositoryBranchesOverviewPage extends React.PureComponent<Props, State> {
    public state: State = {}

    private componentUpdates = new Subject<Props>()
    private subscriptions = new Subscription()

    public componentDidMount(): void {
        eventLogger.logViewEvent('RepositoryBranchesOverview')

        this.subscriptions.add(
            this.componentUpdates
                .pipe(
                    distinctUntilChanged((a, b) => a.repo.id === b.repo.id),
                    switchMap(({ repo }) => {
                        type PartialStateUpdate = Pick<State, 'dataOrError'>
                        const result = fetchGitBranches({ repo: repo.id, first: 10 }).pipe(
                            catchError(error => [error]),
                            map(c => ({ dataOrError: c } as PartialStateUpdate))
                        )
                        return merge(of({ dataOrError: undefined }), result)
                    })
                )
                .subscribe(stateUpdate => this.setState(stateUpdate), error => console.error(error))
        )
        this.componentUpdates.next(this.props)
    }

    public componentWillUpdate(nextProps: Props): void {
        this.componentUpdates.next(nextProps)
    }

    public componentWillUnmount(): void {
        this.subscriptions.unsubscribe()
    }

    public render(): JSX.Element | null {
        return (
            <div className="repository-branches-page">
                {this.state.dataOrError === undefined ? (
                    <LoaderIcon className="icon-inline mt-2" />
                ) : isErrorLike(this.state.dataOrError) ? (
                    <div className="alert alert-danger mt-2">Error: {upperFirst(this.state.dataOrError.message)}</div>
                ) : (
                    <div className="repository-branches-page__cards">
                        {this.state.dataOrError.defaultBranch && (
                            <div className="card repository-branches-page__card">
                                <div className="card-header">Default branch</div>
                                <ul className="list-group list-group-flush">
                                    <GitBranchNode node={this.state.dataOrError.defaultBranch} />
                                </ul>
                            </div>
                        )}
                        {this.state.dataOrError.activeBranches.length > 0 && (
                            <div className="card repository-branches-page__card">
                                <div className="card-header">Active branches</div>
                                <div className="list-group list-group-flush">
                                    {this.state.dataOrError.activeBranches.map((b, i) => (
                                        <GitBranchNode key={i} node={b} />
                                    ))}
                                    {this.state.dataOrError.hasMoreActiveBranches && (
                                        <Link
                                            className="list-group-item list-group-item-action py-2 d-flex"
                                            to={`/${this.props.repo.uri}/-/branches/all`}
                                        >
                                            View more branches<ChevronRight className="icon-inline" />
                                        </Link>
                                    )}
                                </div>
                            </div>
                        )}
                    </div>
                )}
            </div>
        )
    }
}
