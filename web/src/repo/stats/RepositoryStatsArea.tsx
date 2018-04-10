import DirectionalSignIcon from '@sourcegraph/icons/lib/DirectionalSign'
import * as React from 'react'
import { Route, RouteComponentProps, Switch } from 'react-router'
import { Subscription } from 'rxjs/Subscription'
import { HeroPage } from '../../components/HeroPage'
import { RepoHeaderActionPortal } from '../RepoHeaderActionPortal'
import { RepoHeaderBreadcrumbNavItem } from '../RepoHeaderBreadcrumbNavItem'
import { RepositoryStatsContributorsPage } from './RepositoryStatsContributorsPage'
import { RepositoryStatsNavbar } from './RepositoryStatsNavbar'

const NotFoundPage = () => (
    <HeroPage
        icon={DirectionalSignIcon}
        title="404: Not Found"
        subtitle="Sorry, the requested repository stats page was not found."
    />
)

interface Props extends RouteComponentProps<{}> {
    repo: GQL.IRepository
}

/**
 * Properties passed to all page components in the repository stats area.
 */
export interface RepositoryStatsAreaPageProps {
    /**
     * The active repository.
     */
    repo: GQL.IRepository
}

/**
 * Renders pages related to repository stats.
 */
export class RepositoryStatsArea extends React.Component<Props> {
    private subscriptions = new Subscription()

    public componentWillUnmount(): void {
        this.subscriptions.unsubscribe()
    }

    public render(): JSX.Element | null {
        const transferProps: RepositoryStatsAreaPageProps = {
            repo: this.props.repo,
        }

        return (
            <div className="repository-stats-area area--vertical">
                <RepoHeaderActionPortal
                    position="nav"
                    element={<RepoHeaderBreadcrumbNavItem key="stats">Stats</RepoHeaderBreadcrumbNavItem>}
                />
                <div className="area--vertical__navbar">
                    <RepositoryStatsNavbar className="area--vertical__navbar-inner" repo={this.props.repo.uri} />
                </div>
                <div className="area--vertical__content">
                    <div className="area--vertical__content-inner">
                        <Switch>
                            <Route
                                path={`${this.props.match.url}/contributors`}
                                key="hardcoded-key" // see https://github.com/ReactTraining/react-router/issues/4578#issuecomment-334489490
                                exact={true}
                                // tslint:disable-next-line:jsx-no-lambda
                                render={routeComponentProps => (
                                    <RepositoryStatsContributorsPage {...routeComponentProps} {...transferProps} />
                                )}
                            />
                            <Route key="hardcoded-key" component={NotFoundPage} />
                        </Switch>
                    </div>
                </div>
            </div>
        )
    }
}
