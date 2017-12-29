import format from 'date-fns/format'
import * as React from 'react'
import { RouteComponentProps } from 'react-router'
import { Link } from 'react-router-dom'
import { Subscription } from 'rxjs/Subscription'
import { PageTitle } from '../components/PageTitle'
import { eventLogger } from '../tracking/eventLogger'
import { pluralize } from '../util/strings'
import { fetchAllRepositories } from './backend'

interface Props extends RouteComponentProps<any> {}

export interface State {
    repos?: GQL.IRepository[]
    totalCount?: number
}

/**
 * A page displaying the repositories on this site.
 */
export class SiteAdminRepositoriesPage extends React.Component<Props, State> {
    public state: State = {}

    private subscriptions = new Subscription()

    public componentDidMount(): void {
        eventLogger.logViewEvent('SiteAdminRepos')

        this.subscriptions.add(
            fetchAllRepositories().subscribe(resp =>
                this.setState({
                    repos: resp.nodes,
                    totalCount: resp.totalCount,
                })
            )
        )
    }

    public componentWillUnmount(): void {
        this.subscriptions.unsubscribe()
    }

    public render(): JSX.Element | null {
        return (
            <div className="site-admin-detail-list site-admin-repositories-page">
                <PageTitle title="Repositories" />
                <h2>
                    Repositories{' '}
                    {typeof this.state.totalCount === 'number' &&
                        this.state.totalCount > 0 &&
                        `(${this.state.totalCount})`}
                </h2>
                <p>
                    See{' '}
                    <a href="https://about.sourcegraph.com/docs/server/config/repositories">
                        Sourcegraph documentation
                    </a>{' '}
                    for information about adding repositories and integrating with code hosts.
                </p>
                <ul className="site-admin-detail-list__list">
                    {this.state.repos &&
                        this.state.repos.map(repo => (
                            <li
                                key={repo.id}
                                className="site-admin-detail-list__item site-admin-repositories-page__repo"
                            >
                                <div className="site-admin-detail-list__header site-admin-repositories-page__repo-header">
                                    <Link to={`/${repo.uri}`} className="site-admin-detail-list__name">
                                        {repo.uri}
                                    </Link>
                                </div>
                                <ul className="site-admin-detail-list__info site-admin-repositories-page__repo-info">
                                    <li>ID: {repo.id}</li>
                                    {repo.createdAt && <li>Created: {format(repo.createdAt, 'YYYY-MM-DD')}</li>}
                                </ul>
                            </li>
                        ))}
                </ul>
                {this.state.repos &&
                    typeof this.state.totalCount === 'number' &&
                    this.state.totalCount > 0 && (
                        <p>
                            <small>
                                {this.state.totalCount} {pluralize('repository', this.state.totalCount, 'repositories')}{' '}
                                total{' '}
                                {this.state.repos.length < this.state.totalCount &&
                                    `(showing first ${this.state.repos.length})`}
                            </small>
                        </p>
                    )}
            </div>
        )
    }
}
