import AddIcon from '@sourcegraph/icons/lib/Add'
import CircleCheckmarkIcon from '@sourcegraph/icons/lib/CircleCheckmark'
import DirectionalSignIcon from '@sourcegraph/icons/lib/DirectionalSign'
import Loader from '@sourcegraph/icons/lib/Loader'
import NoEntryIcon from '@sourcegraph/icons/lib/NoEntry'
import { upperFirst } from 'lodash'
import * as React from 'react'
import { merge } from 'rxjs/observable/merge'
import { of } from 'rxjs/observable/of'
import { catchError } from 'rxjs/operators/catchError'
import { delay } from 'rxjs/operators/delay'
import { distinctUntilChanged } from 'rxjs/operators/distinctUntilChanged'
import { map } from 'rxjs/operators/map'
import { switchMap } from 'rxjs/operators/switchMap'
import { withLatestFrom } from 'rxjs/operators/withLatestFrom'
import { Subject } from 'rxjs/Subject'
import { Subscription } from 'rxjs/Subscription'
import * as GQL from '../backend/graphqlschema'
import { HeroPage } from '../components/HeroPage'
import { addRepository, checkMirrorRepositoryConnection, setRepositoryEnabled } from '../site-admin/backend'
import { eventLogger } from '../tracking/eventLogger'
import { asError, ErrorLike, isErrorLike } from '../util/errors'

interface Props {
    /** The name of the repository. */
    repo: string

    /** The GraphQL ID of the repository, or null if it doesn't exist. */
    repoID: GQLID | null

    /** The error that occurred while (unsuccessfully) retrieving the repository, or 'disabled' if
     *  the repository is disabled.
     */
    error: ErrorLike | 'disabled'

    /** Whether the viewer is a site admin. */
    viewerCanAdminister: boolean

    /** Called when the repository is successfully added. */
    onDidAddRepository?: () => void

    /** Called when the repository is successfully enabled. */
    onDidUpdateRepository?: (update: Partial<GQL.IRepository>) => void
}

interface State {
    /**
     * Whether the option to add the repository should be shown.
     */
    showAdd: boolean

    /**
     * Whether the site admin can add this repository. undefined while loading.
     */
    canAddOrError?: boolean | ErrorLike

    /**
     * Whether the repository was added successfully. undefined before being triggered, 'loading' while loading,
     * true if successful, and an error otherwise.
     */
    addedOrError?: true | 'loading' | ErrorLike

    /**
     * Whether the option to enable the repository should be shown.
     */
    showEnable: boolean

    /**
     * Whether the repository was enabled successfully. undefined before being triggered, 'loading' while loading,
     * true if successful, and an error otherwise.
     */
    enabledOrError?: true | 'loading' | ErrorLike
}

/**
 * A page informing the user that an error occurred while trying to display the repository. It
 * attempts to present the user with actions to solve the problem.
 */
export class RepositoryErrorPage extends React.PureComponent<Props, State> {
    public state: State = {
        showAdd: false,
        showEnable: false,
    }

    private componentUpdates = new Subject<Props>()
    private addClicks = new Subject<void>()
    private enableClicks = new Subject<void>()
    private subscriptions = new Subscription()

    public componentDidMount(): void {
        eventLogger.logViewEvent('RepositoryError')

        // Show/hide add.
        this.subscriptions.add(
            this.componentUpdates
                .pipe(
                    distinctUntilChanged(
                        (a, b) =>
                            a.repo === b.repo && a.error === b.error && a.viewerCanAdminister === b.viewerCanAdminister
                    ),
                    switchMap(({ repo, error, viewerCanAdminister }) => {
                        type PartialStateUpdate = Pick<State, 'showAdd' | 'canAddOrError'>
                        if (error === 'disabled' || !viewerCanAdminister) {
                            return of({ showAdd: false, canAddOrError: undefined })
                        }
                        return merge<PartialStateUpdate>(
                            of({ showAdd: true, canAddOrError: undefined }),
                            checkMirrorRepositoryConnection({ name: repo }).pipe(
                                map(c => c.error === null),
                                catchError(error => [asError(error)]),
                                map(c => ({ canAddOrError: c } as PartialStateUpdate))
                            )
                        )
                    })
                )
                .subscribe(stateUpdate => this.setState(stateUpdate), error => console.error(error))
        )

        // Handle add.
        this.subscriptions.add(
            this.addClicks
                .pipe(
                    withLatestFrom(this.componentUpdates),
                    switchMap(([, { repo }]) =>
                        merge<Pick<State, 'addedOrError'>>(
                            of({ addedOrError: 'loading' }),
                            addRepository(repo).pipe(
                                switchMap(({ id }) => setRepositoryEnabled(id, true)),
                                map(c => true),

                                // HACK: Delay for gitserver to report the repository as cloning (after
                                // the call to setRepositoryEnabled above, which will trigger a clone).
                                // Without this, there is a race condition where immediately after
                                // clicking this enable button, gitserver reports revision-not-found and
                                // not cloning-in-progress. We need it to report cloning-in-progress so
                                // that the browser polls for the clone to be complete.
                                //
                                // See https://github.com/sourcegraph/sourcegraph/pull/9304.
                                delay(1500),

                                catchError(error => [asError(error)]),
                                map(c => ({ addedOrError: c } as Pick<State, 'addedOrError'>))
                            )
                        )
                    )
                )
                .subscribe(
                    stateUpdate => {
                        this.setState(stateUpdate)
                        if (this.props.onDidAddRepository && stateUpdate.addedOrError === true) {
                            this.props.onDidAddRepository()
                        }
                    },
                    error => console.error(error)
                )
        )

        // Show/hide enable.
        this.subscriptions.add(
            this.componentUpdates
                .pipe(
                    distinctUntilChanged(
                        (a, b) =>
                            a.repo === b.repo && a.error === b.error && a.viewerCanAdminister === b.viewerCanAdminister
                    ),
                    map(({ error, viewerCanAdminister }) => ({
                        showEnable: error === 'disabled' && viewerCanAdminister,
                    }))
                )
                .subscribe(stateUpdate => this.setState(stateUpdate), error => console.error(error))
        )

        // Handle enable.
        this.subscriptions.add(
            this.enableClicks
                .pipe(
                    withLatestFrom(this.componentUpdates),
                    switchMap(([, { repoID }]) =>
                        merge<Pick<State, 'enabledOrError'>>(
                            of({ enabledOrError: 'loading' }),
                            setRepositoryEnabled(repoID!, true).pipe(
                                map(c => true),

                                // HACK: Delay for gitserver to report the repository as cloning (after
                                // the call to setRepositoryEnabled above, which will trigger a clone).
                                // Without this, there is a race condition where immediately after
                                // clicking this enable button, gitserver reports revision-not-found and
                                // not cloning-in-progress. We need it to report cloning-in-progress so
                                // that the browser polls for the clone to be complete.
                                //
                                // See https://github.com/sourcegraph/sourcegraph/pull/9304.
                                delay(1500),

                                catchError(error => [asError(error)]),
                                map(c => ({ enabledOrError: c } as Pick<State, 'enabledOrError'>))
                            )
                        )
                    )
                )
                .subscribe(
                    stateUpdate => {
                        this.setState(stateUpdate)

                        if (this.props.onDidUpdateRepository && stateUpdate.enabledOrError === true) {
                            this.props.onDidUpdateRepository({ enabled: true })
                        }
                    },
                    error => console.error(error)
                )
        )

        this.componentUpdates.next(this.props)
    }

    public componentWillReceiveProps(props: Props): void {
        this.componentUpdates.next(props)
    }

    public componentWillUnmount(): void {
        this.subscriptions.unsubscribe()
    }

    public render(): JSX.Element | null {
        let title: string
        let Icon: React.ComponentType
        if (this.props.error === 'disabled') {
            title = 'Repository disabled'
            Icon = NoEntryIcon
        } else {
            title = 'Repository not found'
            Icon = DirectionalSignIcon
        }
        return (
            <HeroPage
                icon={Icon}
                title={title}
                subtitle={
                    <div className="repository-error-page">
                        {this.state.showAdd && (
                            <div className="repository-error-page__section mt-3">
                                <div className="repository-error-page__section-inner">
                                    <div className="repository-error-page__section-description">
                                        {this.state.canAddOrError === undefined && (
                                            <>Checking whether this repository can be added...</>
                                        )}
                                        {(this.state.canAddOrError === false ||
                                            isErrorLike(this.state.canAddOrError)) && (
                                            <>
                                                The repository can't be added because it doesn't exist on any code hosts
                                                configured on this site.
                                            </>
                                        )}
                                        {this.state.canAddOrError === true && (
                                            <>
                                                As a site admin, you can add this repository to Sourcegraph to allow
                                                users to search and view it.
                                            </>
                                        )}
                                    </div>
                                    <div className="repository-error-page__section-action">
                                        <button
                                            className="btn btn-primary repository-error-page__btn"
                                            onClick={this.addRepository}
                                            disabled={
                                                this.state.canAddOrError !== true ||
                                                this.state.addedOrError === 'loading'
                                            }
                                        >
                                            {this.state.canAddOrError === undefined ||
                                            this.state.addedOrError === 'loading' ? (
                                                <Loader className="icon-inline" />
                                            ) : (
                                                <AddIcon className="icon-inline" />
                                            )}{' '}
                                            Add repository
                                        </button>
                                    </div>
                                </div>
                                {isErrorLike(this.state.addedOrError) && (
                                    <div className="alert alert-danger repository-error-page__alert mt-2">
                                        Error adding repository: {upperFirst(this.state.addedOrError.message)}
                                    </div>
                                )}
                            </div>
                        )}
                        {this.state.showEnable && (
                            <div className="repository-error-page__section mt-3">
                                <div className="repository-error-page__section-inner">
                                    <div className="repository-error-page__section-description">
                                        As a site admin, you can enable this repository to allow users to search and
                                        view it.
                                    </div>
                                    <div className="repository-error-page__section-action">
                                        <button
                                            className="btn btn-primary repository-error-page__btn"
                                            onClick={this.enableRepository}
                                            disabled={this.state.enabledOrError === 'loading'}
                                        >
                                            {this.state.enabledOrError === 'loading' ? (
                                                <Loader className="icon-inline" />
                                            ) : (
                                                <CircleCheckmarkIcon className="icon-inline" />
                                            )}{' '}
                                            Enable repository
                                        </button>
                                    </div>
                                </div>
                                {isErrorLike(this.state.enabledOrError) && (
                                    <div className="alert alert-danger repository-error-page__alert mt-2">
                                        Error enabling repository: {upperFirst(this.state.enabledOrError.message)}
                                    </div>
                                )}
                            </div>
                        )}
                        {!this.state.showAdd &&
                            !this.state.showEnable && <p>To access this repository, contact the Sourcegraph admin.</p>}
                    </div>
                }
            />
        )
    }

    private addRepository = () => this.addClicks.next()
    private enableRepository = () => this.enableClicks.next()
}
