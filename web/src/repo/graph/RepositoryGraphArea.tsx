import DirectionalSignIcon from '@sourcegraph/icons/lib/DirectionalSign'
import ErrorIcon from '@sourcegraph/icons/lib/Error'
import { upperFirst } from 'lodash'
import * as React from 'react'
import { Route, RouteComponentProps, Switch } from 'react-router'
import { Subscription } from 'rxjs'
import * as GQL from '../../backend/graphqlschema'
import { DismissibleAlert } from '../../components/DismissibleAlert'
import { HeroPage } from '../../components/HeroPage'
import { RepoHeaderActionPortal } from '../RepoHeaderActionPortal'
import { RepoHeaderBreadcrumbNavItem } from '../RepoHeaderBreadcrumbNavItem'
import { RepositoryGraphDependenciesPage } from './RepositoryGraphDependenciesPage'
import { RepositoryGraphOverviewPage } from './RepositoryGraphOverviewPage'
import { RepositoryGraphPackagesPage } from './RepositoryGraphPackagesPage'
import { RepositoryGraphSidebar } from './RepositoryGraphSidebar'

const NotFoundPage = () => (
    <HeroPage
        icon={DirectionalSignIcon}
        title="404: Not Found"
        subtitle="Sorry, the requested repository graph page was not found."
    />
)

interface Props extends RouteComponentProps<{}> {
    repo: GQL.IRepository
    rev: string | undefined
    commitID: string
    defaultBranch: string
    user: GQL.IUser | null
    routePrefix: string
}

interface State {
    error?: string
}

/**
 * Renders a layout of a sidebar and a content area to display pages related to
 * the repository graph.
 */
export class RepositoryGraphArea extends React.Component<Props> {
    public state: State = {}

    private subscriptions = new Subscription()

    public componentWillUnmount(): void {
        this.subscriptions.unsubscribe()
    }

    public render(): JSX.Element | null {
        if (this.state.error) {
            return <HeroPage icon={ErrorIcon} title="Error" subtitle={upperFirst(this.state.error)} />
        }
        if (!this.props.user) {
            return null
        }

        const transferProps: { user: GQL.IUser; repo: GQL.IRepository; rev: string | undefined; commitID: string } = {
            user: this.props.user,
            repo: this.props.repo,
            rev: this.props.rev,
            commitID: this.props.commitID,
        }

        return (
            <div className="repository-graph-area area">
                <RepoHeaderActionPortal
                    position="nav"
                    element={<RepoHeaderBreadcrumbNavItem key="graph">Graph</RepoHeaderBreadcrumbNavItem>}
                />
                <RepositoryGraphSidebar
                    className="area__sidebar"
                    {...this.props}
                    {...transferProps}
                    routePrefix={this.props.routePrefix}
                />
                <div className="area__content">
                    <DismissibleAlert className="alert-warning mb-4" partialStorageKey="repository-graph-experimental">
                        <span>
                            The repository graph area is an <strong>experimental</strong> feature that lets you explore
                            a repository's dependencies and packages. Not all languages and repositories are supported.
                        </span>
                    </DismissibleAlert>
                    <Switch>
                        <Route
                            path={`${this.props.match.url}`}
                            key="hardcoded-key" // see https://github.com/ReactTraining/react-router/issues/4578#issuecomment-334489490
                            exact={true}
                            // tslint:disable-next-line:jsx-no-lambda
                            render={routeComponentProps => (
                                <RepositoryGraphOverviewPage
                                    {...routeComponentProps}
                                    {...transferProps}
                                    routePrefix={this.props.routePrefix}
                                />
                            )}
                        />
                        <Route
                            path={`${this.props.match.url}/packages`}
                            key="hardcoded-key" // see https://github.com/ReactTraining/react-router/issues/4578#issuecomment-334489490
                            exact={true}
                            // tslint:disable-next-line:jsx-no-lambda
                            render={routeComponentProps => (
                                <RepositoryGraphPackagesPage {...routeComponentProps} {...transferProps} />
                            )}
                        />
                        <Route
                            path={`${this.props.match.url}/dependencies`}
                            key="hardcoded-key" // see https://github.com/ReactTraining/react-router/issues/4578#issuecomment-334489490
                            exact={true}
                            // tslint:disable-next-line:jsx-no-lambda
                            render={routeComponentProps => (
                                <RepositoryGraphDependenciesPage {...routeComponentProps} {...transferProps} />
                            )}
                        />
                        <Route key="hardcoded-key" component={NotFoundPage} />
                    </Switch>
                </div>
            </div>
        )
    }
}
