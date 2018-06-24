import BranchIcon from '@sourcegraph/icons/lib/Branch'
import CommitIcon from '@sourcegraph/icons/lib/Commit'
import { Folder as FolderIcon } from '@sourcegraph/icons/lib/Folder'
import HistoryIcon from '@sourcegraph/icons/lib/History'
import { Loader } from '@sourcegraph/icons/lib/Loader'
import { Repo as RepositoryIcon } from '@sourcegraph/icons/lib/Repo'
import TagIcon from '@sourcegraph/icons/lib/Tag'
import UserIcon from '@sourcegraph/icons/lib/User'
import escapeRegexp from 'escape-string-regexp'
import * as H from 'history'
import { upperFirst } from 'lodash'
import * as React from 'react'
import { Link } from 'react-router-dom'
import { Observable, Subject, Subscription } from 'rxjs'
import { catchError, distinctUntilChanged, map, startWith, switchMap, tap } from 'rxjs/operators'
import { gql, queryGraphQL } from '../backend/graphql'
import * as GQL from '../backend/graphqlschema'
import { Form } from '../components/Form'
import { PageTitle } from '../components/PageTitle'
import { displayRepoPath } from '../components/RepoFileLink'
import { OpenHelpPopoverButton } from '../global/OpenHelpPopoverButton'
import { searchQueryForRepoRev } from '../search'
import { submitSearch } from '../search/helpers'
import { QueryInput } from '../search/input/QueryInput'
import { SearchButton } from '../search/input/SearchButton'
import { eventLogger } from '../tracking/eventLogger'
import { asError, createAggregateError, ErrorLike, isErrorLike } from '../util/errors'
import { memoizeObservable } from '../util/memoize'
import { basename } from '../util/path'
import { fetchTree } from './backend'
import { GitCommitNode } from './commits/GitCommitNode'
import { FilteredGitCommitConnection, gitCommitFragment } from './commits/RepositoryCommitsPage'

const TreeEntry: React.SFC<{
    isDir: boolean
    name: string
    parentPath: string
    url: string
}> = ({ isDir, name, parentPath, url }) => {
    const filePath = parentPath ? parentPath + '/' + name : name
    return (
        <Link to={url} className="tree-entry" title={filePath}>
            {name}
            {isDir && '/'}
        </Link>
    )
}

const fetchTreeCommits = memoizeObservable(
    (args: {
        repo: GQL.ID
        revspec: string
        first?: number
        filePath?: string
    }): Observable<GQL.IGitCommitConnection> =>
        queryGraphQL(
            gql`
                query TreeCommits($repo: ID!, $revspec: String!, $first: Int, $filePath: String) {
                    node(id: $repo) {
                        ... on Repository {
                            commit(rev: $revspec) {
                                ancestors(first: $first, path: $filePath) {
                                    nodes {
                                        ...GitCommitFields
                                    }
                                    pageInfo {
                                        hasNextPage
                                    }
                                }
                            }
                        }
                    }
                }
                ${gitCommitFragment}
            `,
            args
        ).pipe(
            map(({ data, errors }) => {
                if (!data || !data.node) {
                    throw createAggregateError(errors)
                }
                const repo = data.node as GQL.IRepository
                if (!repo.commit || !repo.commit.ancestors || !repo.commit.ancestors.nodes) {
                    throw createAggregateError(errors)
                }
                return repo.commit.ancestors
            })
        ),
    args => `${args.repo}:${args.revspec}:${args.first}:${args.filePath}`
)

interface Props {
    repoPath: string
    repoID: GQL.ID
    repoDescription: string
    // filePath is the tree's path in TreePage. We call it filePath for consistency elsewhere.
    filePath: string
    commitID: string
    rev: string
    isLightTheme: boolean
    onHelpPopoverToggle: () => void

    location: H.Location
    history: H.History
}

interface State {
    /** This tree, or an error. Undefined while loading. */
    treeOrError?: GQL.IGitTree | ErrorLike

    /**
     * The value of the search query input field.
     */
    query: string
}

export class TreePage extends React.PureComponent<Props, State> {
    public state: State = { query: '' }

    private componentUpdates = new Subject<Props>()
    private subscriptions = new Subscription()

    private logViewEvent(props: Props): void {
        if (props.filePath === '') {
            eventLogger.logViewEvent('Repository')
        } else {
            eventLogger.logViewEvent('Tree')
        }
    }

    public componentDidMount(): void {
        this.subscriptions.add(
            this.componentUpdates
                .pipe(
                    distinctUntilChanged(
                        (x, y) =>
                            x.repoPath === y.repoPath &&
                            x.rev === y.rev &&
                            x.commitID === y.commitID &&
                            x.filePath === y.filePath
                    ),
                    tap(props => this.logViewEvent(props)),
                    switchMap(props =>
                        fetchTree({
                            repoPath: props.repoPath,
                            commitID: props.commitID,
                            rev: props.rev,
                            filePath: props.filePath,
                            first: 2500,
                        }).pipe(
                            catchError(err => [asError(err)]),
                            map(c => ({ treeOrError: c })),
                            startWith<Pick<State, 'treeOrError'>>({ treeOrError: undefined })
                        )
                    )
                )
                .subscribe(stateUpdate => this.setState(stateUpdate), err => console.error(err))
        )

        this.componentUpdates.next(this.props)
    }

    public componentWillReceiveProps(newProps: Props): void {
        this.componentUpdates.next(newProps)
    }

    public componentWillUnmount(): void {
        this.subscriptions.unsubscribe()
    }

    private getQueryPrefix(): string {
        let queryPrefix = searchQueryForRepoRev(this.props.repoPath, this.props.rev)
        if (this.props.filePath) {
            queryPrefix += `file:^${escapeRegexp(this.props.filePath)}/ `
        }
        return queryPrefix
    }

    public render(): JSX.Element | null {
        return (
            <div className="tree-page">
                <PageTitle title={this.getPageTitle()} />
                {this.state.treeOrError === undefined && (
                    <div>
                        <Loader className="icon-inline tree-page__entries-loader" /> Loading files and directories
                    </div>
                )}
                {this.state.treeOrError !== undefined &&
                    (isErrorLike(this.state.treeOrError) ? (
                        <div className="alert alert-danger">{upperFirst(this.state.treeOrError.message)}</div>
                    ) : (
                        <>
                            {this.state.treeOrError.isRoot ? (
                                <header>
                                    <h2 className="tree-page__title">
                                        <RepositoryIcon className="icon-inline" />{' '}
                                        {displayRepoPath(this.props.repoPath)}
                                    </h2>
                                    {this.props.repoDescription && <p>{this.props.repoDescription}</p>}
                                    <div className="btn-group mb-3">
                                        <Link
                                            className="btn btn-secondary"
                                            to={`${this.state.treeOrError.url}/-/commits`}
                                        >
                                            <CommitIcon className="icon-inline" /> Commits
                                        </Link>
                                        <Link className="btn btn-secondary" to={`/${this.props.repoPath}/-/branches`}>
                                            <BranchIcon className="icon-inline" /> Branches
                                        </Link>
                                        <Link className="btn btn-secondary" to={`/${this.props.repoPath}/-/tags`}>
                                            <TagIcon className="icon-inline" /> Tags
                                        </Link>
                                        <Link
                                            className="btn btn-secondary"
                                            to={
                                                this.props.rev
                                                    ? `/${this.props.repoPath}/-/compare/...${encodeURIComponent(
                                                          this.props.rev
                                                      )}`
                                                    : `/${this.props.repoPath}/-/compare`
                                            }
                                        >
                                            <HistoryIcon className="icon-inline" /> Compare
                                        </Link>
                                        <Link
                                            className={`btn btn-secondary`}
                                            to={`/${this.props.repoPath}/-/stats/contributors`}
                                        >
                                            <UserIcon className="icon-inline" /> Contributors
                                        </Link>
                                    </div>
                                </header>
                            ) : (
                                <header>
                                    <h2 className="tree-page__title">
                                        <FolderIcon className="icon-inline" /> {this.props.filePath}
                                    </h2>
                                </header>
                            )}
                            <section className="tree-page__section">
                                <h3 className="tree-page__section-header">
                                    Search in this {this.props.filePath ? 'tree' : 'repository'}
                                </h3>
                                <Form className="tree-page__section-search" onSubmit={this.onSubmit}>
                                    <QueryInput
                                        value={this.state.query}
                                        onChange={this.onQueryChange}
                                        prependQueryForSuggestions={this.getQueryPrefix()}
                                        autoFocus={true}
                                        location={this.props.location}
                                        history={this.props.history}
                                        placeholder=""
                                    />
                                    <SearchButton />
                                    <OpenHelpPopoverButton onHelpPopoverToggle={this.props.onHelpPopoverToggle} />
                                </Form>
                            </section>
                            {this.state.treeOrError.directories.length > 0 && (
                                <section className="tree-page__section">
                                    <h3 className="tree-page__section-header">Directories</h3>
                                    <div className="tree-page__entries tree-page__entries-directories">
                                        {this.state.treeOrError.directories.map((e, i) => (
                                            <TreeEntry
                                                key={i}
                                                isDir={true}
                                                name={e.name}
                                                parentPath={this.props.filePath}
                                                url={e.url}
                                            />
                                        ))}
                                    </div>
                                </section>
                            )}
                            {this.state.treeOrError.files.length > 0 && (
                                <section className="tree-page__section">
                                    <h3 className="tree-page__section-header">Files</h3>
                                    <div className="tree-page__entries tree-page__entries-files">
                                        {this.state.treeOrError.files.map((e, i) => (
                                            <TreeEntry
                                                key={i}
                                                isDir={false}
                                                name={e.name}
                                                parentPath={this.props.filePath}
                                                url={e.url}
                                            />
                                        ))}
                                    </div>
                                </section>
                            )}
                            <div className="tree-page__section">
                                <h3 className="tree-page__section-header">Changes</h3>
                                <FilteredGitCommitConnection
                                    className="mt-2 tree-page__section--commits"
                                    listClassName="list-group list-group-flush"
                                    noun="commit in this tree"
                                    pluralNoun="commits in this tree"
                                    queryConnection={this.queryCommits}
                                    nodeComponent={GitCommitNode}
                                    nodeComponentProps={{
                                        repoName: this.props.repoPath,
                                        className: 'list-group-item',
                                        compact: true,
                                    }}
                                    updateOnChange={`${this.props.repoPath}:${this.props.rev}:${this.props.filePath}`}
                                    defaultFirst={7}
                                    history={this.props.history}
                                    shouldUpdateURLQuery={false}
                                    hideSearch={true}
                                    location={this.props.location}
                                />
                            </div>
                        </>
                    ))}
            </div>
        )
    }

    private onQueryChange = (query: string) => this.setState({ query })

    private onSubmit = (event: React.FormEvent<HTMLFormElement>): void => {
        event.preventDefault()
        submitSearch(
            this.props.history,
            { query: this.getQueryPrefix() + this.state.query },
            this.props.filePath ? 'tree' : 'repo'
        )
    }

    private getPageTitle(): string {
        const repoStr = displayRepoPath(this.props.repoPath)
        if (this.props.filePath) {
            return `${basename(this.props.filePath)} - ${repoStr}`
        }
        return `${repoStr}`
    }

    private queryCommits = (args: { first?: number }) =>
        fetchTreeCommits({
            ...args,
            repo: this.props.repoID,
            revspec: this.props.rev || '',
            filePath: this.props.filePath,
        })
}
