import LoaderIcon from '@sourcegraph/icons/lib/Loader'
import * as React from 'react'
import { RouteComponentProps } from 'react-router'
import { Link } from 'react-router-dom'
import { switchMap } from 'rxjs/operators/switchMap'
import { Subject } from 'rxjs/Subject'
import { Subscription } from 'rxjs/Subscription'
import { REPO_DELETE_CONFIRMATION_MESSAGE } from '.'
import { PageTitle } from '../../components/PageTitle'
import { deleteRepository, setRepositoryEnabled } from '../../site-admin/backend'
import { eventLogger } from '../../tracking/eventLogger'
import { fetchRepository } from './backend'
import { ActionContainer } from './components/ActionContainer'

interface Props extends RouteComponentProps<any> {
    repo: GQL.IRepository
    user: GQL.IUser
    onDidUpdateRepository: (update: Partial<GQL.IRepository>) => void
}

interface State {
    /**
     * The repository object, refreshed after we make changes that modify it.
     */
    repo: GQL.IRepository

    loading: boolean
    error?: string
}

/**
 * The repository settings options page.
 */
export class RepoSettingsOptionsPage extends React.PureComponent<Props, State> {
    private repoUpdates = new Subject<void>()
    private subscriptions = new Subscription()

    constructor(props: Props) {
        super(props)

        this.state = {
            loading: false,
            repo: props.repo,
        }
    }

    public componentDidMount(): void {
        eventLogger.logViewEvent('RepoSettings')

        this.subscriptions.add(
            this.repoUpdates
                .pipe(switchMap(() => fetchRepository(this.props.repo.uri)))
                .subscribe(repo => this.setState({ repo }), err => this.setState({ error: err.message }))
        )
    }

    public componentWillUnmount(): void {
        this.subscriptions.unsubscribe()
    }

    public render(): JSX.Element | null {
        return (
            <div className="repo-settings-options-page">
                <PageTitle title="Repository settings" />
                <h2>Settings</h2>
                {this.state.loading && <LoaderIcon className="icon-inline" />}
                {this.state.error && <div className="alert alert-danger">{this.state.error}</div>}
                <form className="settings-page__form">
                    <div className="form-group">
                        <label>Repository name</label>
                        <input
                            type="text"
                            className="form-control"
                            readOnly={true}
                            disabled={true}
                            value={this.state.repo.uri}
                            required={true}
                            spellCheck={false}
                            autoCapitalize="off"
                            autoCorrect="off"
                        />
                        <small className="form-text">
                            This repository's name is set by its{' '}
                            {this.state.repo.viewerCanAdminister ? (
                                <Link to="/site-admin/configuration">code host configuration</Link>
                            ) : (
                                'code host configuration'
                            )}{' '}
                            and can't be changed.
                        </small>
                        <button className="btn btn-primary mt-1" disabled={true} type="submit">
                            Rename
                        </button>
                    </div>
                </form>
                <ActionContainer
                    title={this.state.repo.enabled ? 'Disable access' : 'Enable access'}
                    description={
                        this.state.repo.enabled
                            ? 'Disable access to the repository to prevent users from searching and browsing the repository.'
                            : 'The repository is disabled. Enable it to allow users to search and view the repository.'
                    }
                    buttonClassName={this.state.repo.enabled ? 'btn-danger' : 'btn-success'}
                    buttonLabel={this.state.repo.enabled ? 'Disable access' : 'Enable access'}
                    flashText="Updated"
                    run={this.state.repo.enabled ? this.disableRepository : this.enableRepository}
                />
                <ActionContainer
                    title="Delete repository"
                    description="Permanently removes this repository and all associated data from Sourcegraph. The original repository on the code host is not affected. If this repository was added by a configured code host, then it will be re-added during the next sync."
                    buttonClassName="btn-danger"
                    buttonLabel="Delete this repository"
                    run={this.deleteRepository}
                />
            </div>
        )
    }

    private enableRepository = () =>
        setRepositoryEnabled(this.state.repo.id, true)
            .toPromise()
            .then(() => {
                this.repoUpdates.next()
                this.props.onDidUpdateRepository({ enabled: true })
            })
    private disableRepository = () =>
        setRepositoryEnabled(this.state.repo.id, false)
            .toPromise()
            .then(() => {
                this.repoUpdates.next()
                this.props.onDidUpdateRepository({ enabled: false })
            })

    private deleteRepository = () => {
        if (!window.confirm(REPO_DELETE_CONFIRMATION_MESSAGE)) {
            return Promise.resolve()
        }

        return deleteRepository(this.state.repo.id)
            .toPromise()
            .then(() => {
                this.repoUpdates.next()
                this.props.history.push('/explore')
            })
    }
}
