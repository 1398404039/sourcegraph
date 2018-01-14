import * as H from 'history'
import * as React from 'react'
import { distinctUntilChanged } from 'rxjs/operators/distinctUntilChanged'
import { skip } from 'rxjs/operators/skip'
import { startWith } from 'rxjs/operators/startWith'
import { Subject } from 'rxjs/Subject'
import { Subscription } from 'rxjs/Subscription'
import { submitSearch } from './helpers'
import { parseSearchURLQuery, SearchOptions, searchOptionsEqual } from './index'
import { QueryInput } from './QueryInput'
import { SearchButton } from './SearchButton'
import { SearchHelp } from './SearchHelp'

interface Props {
    location: H.Location
    history: H.History
}

interface State {
    /** The query in the input field */
    userQuery: string
}

/**
 * The search item in the navbar
 */
export class SearchNavbarItem extends React.Component<Props, State> {
    /** Subscriptions to unsubscribe from on component unmount */
    private subscriptions = new Subscription()

    /** Emits on componentWillReceiveProps */
    private componentUpdates = new Subject<Props>()

    constructor(props: Props) {
        super(props)

        // Fill text input from URL info
        this.state = this.getStateFromProps(props) || { userQuery: '' }

        /** Emits whenever the route changes */
        const routeChanges = this.componentUpdates.pipe(
            startWith(props),
            distinctUntilChanged((a, b) => a.location === b.location),
            skip(1)
        )

        // Reset on route changes
        this.subscriptions.add(
            routeChanges.subscribe(
                props => {
                    this.setState(this.getStateFromProps(props))
                },
                err => {
                    console.error(err)
                }
            )
        )

        // Listen to location changes in both ways. Depending on the source of the
        // history event, it might be seen first by one or the other. If we don't
        // listen for both, then we might receive some events too late.
        this.subscriptions.add(routeChanges.subscribe(props => this.onLocationChange(props.location)))
        this.subscriptions.add(props.history.listen(location => this.onLocationChange(location)))
    }

    public componentWillReceiveProps(newProps: Props): void {
        this.componentUpdates.next(newProps)
    }

    public componentWillUnmount(): void {
        this.subscriptions.unsubscribe()
    }

    public render(): JSX.Element | null {
        // Only autofocus the query input on search result pages (otherwise we
        // capture down-arrow keypresses that the user probably intends to scroll down
        // in the page).
        const autoFocus = this.props.location.pathname === '/search'

        return (
            <form className="search search--navbar-item" onSubmit={this.onSubmit}>
                <QueryInput
                    {...this.props}
                    value={this.state.userQuery}
                    onChange={this.onUserQueryChange}
                    autoFocus={autoFocus ? 'cursor-at-end' : undefined}
                    hasGlobalQueryBehavior={true}
                />
                <SearchButton />
                <SearchHelp />
            </form>
        )
    }

    private onLocationChange = (location: H.Location): void => {
        // Store the last-used search options ('q' and 'sq' query parameters) in the location
        // state if we're navigating to a URL that lacks them, so that we can preserve them without
        // storing them in the URL (which is ugly) and across page reloads in the same tab.
        const oldSearch: SearchOptions = { query: this.state.userQuery }
        const locationStateNeedsUpdate =
            !location.state || !searchOptionsEqual(location.state as SearchOptions, oldSearch)
        const newSearch = parseSearchURLQuery(location.search)
        if (locationStateNeedsUpdate && !newSearch) {
            this.props.history.replace({
                pathname: location.pathname,
                hash: location.hash,
                search: location.search,
                state: { ...location.state, ...oldSearch },
            })
        }
    }

    private onUserQueryChange = (userQuery: string) => {
        this.setState({ userQuery })
    }

    private onSubmit = (event: React.FormEvent<HTMLFormElement>): void => {
        event.preventDefault()
        submitSearch(
            this.props.history,
            {
                query: this.state.userQuery,
            },
            'nav'
        )
    }

    /**
     * Reads initial state from the props (i.e. URL parameters).
     */
    private getStateFromProps(props: Props): State {
        const options = parseSearchURLQuery(props.location.search || '')
        if (options) {
            return { userQuery: options.query }
        }

        // If the new URL has no search options, then preserve the ones we had before.
        // That makes it so that if we navigate from search results to a blob, the
        // query and scope will remain the same (instead of being cleared).
        //
        // The first place to look for the previous query options is in our state.
        if (this.state) {
            return this.state
        }

        // If we have no component state, then we may have gotten unmounted during a route change.
        // We always store the last query in the location state, so check there.
        const state: SearchOptions | undefined = props.location.state
        return {
            userQuery: state ? state.query : '',
        }
    }
}
