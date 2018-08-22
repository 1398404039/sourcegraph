// Polyfill URL because Chrome and Firefox are not spec-compliant
// Hostnames of URIs with custom schemes (e.g. git) are not parsed out
import { URL, URLSearchParams } from 'whatwg-url'
// The polyfill does not expose createObjectURL, which we need for creating data: URIs for Web
// Workers. So retain it.
//
// tslint:disable-next-line:no-unbound-method
const createObjectURL = window.URL ? window.URL.createObjectURL : null
Object.assign(window, { URL, URLSearchParams })
;(window.URL.createObjectURL as any) = createObjectURL

// Load only a subset of the highlight.js languages
import { registerLanguage } from 'highlight.js/lib/highlight'
registerLanguage('go', require('highlight.js/lib/languages/go'))
registerLanguage('javascript', require('highlight.js/lib/languages/javascript'))
registerLanguage('typescript', require('highlight.js/lib/languages/typescript'))
registerLanguage('java', require('highlight.js/lib/languages/java'))
registerLanguage('python', require('highlight.js/lib/languages/python'))
registerLanguage('php', require('highlight.js/lib/languages/php'))
registerLanguage('bash', require('highlight.js/lib/languages/bash'))
registerLanguage('clojure', require('highlight.js/lib/languages/clojure'))
registerLanguage('cpp', require('highlight.js/lib/languages/cpp'))
registerLanguage('cs', require('highlight.js/lib/languages/cs'))
registerLanguage('css', require('highlight.js/lib/languages/css'))
registerLanguage('dockerfile', require('highlight.js/lib/languages/dockerfile'))
registerLanguage('elixir', require('highlight.js/lib/languages/elixir'))
registerLanguage('haskell', require('highlight.js/lib/languages/haskell'))
registerLanguage('html', require('highlight.js/lib/languages/xml'))
registerLanguage('lua', require('highlight.js/lib/languages/lua'))
registerLanguage('ocaml', require('highlight.js/lib/languages/ocaml'))
registerLanguage('r', require('highlight.js/lib/languages/r'))
registerLanguage('ruby', require('highlight.js/lib/languages/ruby'))
registerLanguage('rust', require('highlight.js/lib/languages/rust'))
registerLanguage('swift', require('highlight.js/lib/languages/swift'))

import { Notifications } from '@sourcegraph/extensions-client-common/lib/app/notifications/Notifications'
import { createController as createCXPController } from '@sourcegraph/extensions-client-common/lib/cxp/controller'
import { ConfiguredExtension } from '@sourcegraph/extensions-client-common/lib/extensions/extension'
import {
    ConfigurationCascadeOrError,
    ConfigurationSubject,
    ConfiguredSubject,
    Settings,
} from '@sourcegraph/extensions-client-common/lib/settings'
import ErrorIcon from '@sourcegraph/icons/lib/Error'
import ServerIcon from '@sourcegraph/icons/lib/Server'
import {
    Component as CXPComponent,
    EMPTY_ENVIRONMENT as CXP_EMPTY_ENVIRONMENT,
} from 'cxp/module/environment/environment'
import { URI } from 'cxp/module/types/textDocument'
import * as React from 'react'
import { render } from 'react-dom'
import { Redirect, Route, RouteComponentProps, Switch } from 'react-router'
import { BrowserRouter } from 'react-router-dom'
import { Subscription } from 'rxjs'
import { currentUser } from './auth'
import * as GQL from './backend/graphqlschema'
import { FeedbackText } from './components/FeedbackText'
import { HeroPage } from './components/HeroPage'
import { Tooltip } from './components/tooltip/Tooltip'
import { CXPComponentProps, CXPEnvironmentProps, USE_PLATFORM } from './cxp/CXPEnvironment'
import { CXPRootProps } from './cxp/CXPRoot'
import { LinkExtension } from './extension/Link'
import {
    ConfigurationCascadeProps,
    createMessageTransports,
    CXPControllerProps,
    ExtensionsProps,
} from './extensions/ExtensionsClientCommonContext'
import { createExtensionsContextController } from './extensions/ExtensionsClientCommonContext'
import { GlobalAlerts } from './global/GlobalAlerts'
import { GlobalDebug } from './global/GlobalDebug'
import { IntegrationsToast } from './marketing/IntegrationsToast'
import { updateUserSessionStores } from './marketing/util'
import { GlobalNavbar } from './nav/GlobalNavbar'
import { routes } from './routes'
import { parseSearchURLQuery } from './search'
import { eventLogger } from './tracking/eventLogger'
import { isErrorLike } from './util/errors'

interface LayoutProps
    extends RouteComponentProps<any>,
        ConfigurationCascadeProps,
        ExtensionsProps,
        CXPEnvironmentProps,
        CXPControllerProps,
        CXPComponentProps,
        CXPRootProps {
    user: GQL.IUser | null

    /**
     * The subject GraphQL node ID of the viewer, which is used to look up the viewer's configuration settings.
     * This is either the site's GraphQL node ID (for anonymous users) or the authenticated user's GraphQL node ID.
     */
    viewerSubject: Pick<GQL.IConfigurationSubject, 'id' | 'viewerCanAdminister'>

    isLightTheme: boolean
    onThemeChange: () => void
    navbarSearchQuery: string
    onNavbarQueryChange: (query: string) => void
    showHelpPopover: boolean
    showHistoryPopover: boolean
    onHelpPopoverToggle: (visible?: boolean) => void
    onHistoryPopoverToggle: (visible?: boolean) => void
}

const Layout: React.SFC<LayoutProps> = props => {
    const isSearchHomepage = props.location.pathname === '/search' && !parseSearchURLQuery(props.location.search)

    const needsSiteInit = window.context.showOnboarding
    const isSiteInit = props.location.pathname === '/site-admin/init'

    // Force light theme on site init page.
    if (isSiteInit && !props.isLightTheme) {
        props.onThemeChange()
    }

    // Remove trailing slash (which is never valid in any of our URLs).
    if (props.location.pathname !== '/' && props.location.pathname.endsWith('/')) {
        return <Redirect to={{ ...props.location, pathname: props.location.pathname.slice(0, -1) }} />
    }

    return (
        <div className="layout">
            <GlobalAlerts isSiteAdmin={!!props.user && props.user.siteAdmin} />
            {!needsSiteInit && !isSiteInit && !!props.user && <IntegrationsToast history={props.history} />}
            {!isSiteInit && <GlobalNavbar {...props} lowProfile={isSearchHomepage} />}
            {needsSiteInit && !isSiteInit && <Redirect to="/site-admin/init" />}
            <Switch>
                {routes.map((route, i) => {
                    const isFullWidth = !route.forceNarrowWidth
                    const Component = route.component
                    return (
                        <Route
                            {...route}
                            key="hardcoded-key" // see https://github.com/ReactTraining/react-router/issues/4578#issuecomment-334489490
                            component={undefined}
                            // tslint:disable-next-line:jsx-no-lambda
                            render={routeComponentProps => (
                                <div
                                    className={[
                                        'layout__app-router-container',
                                        `layout__app-router-container--${isFullWidth ? 'full-width' : 'restricted'}`,
                                    ].join(' ')}
                                >
                                    {Component && (
                                        <Component {...props} {...routeComponentProps} isFullWidth={isFullWidth} />
                                    )}
                                    {route.render && route.render({ ...props, ...routeComponentProps })}
                                    {!!props.user && <LinkExtension user={props.user} />}
                                </div>
                            )}
                        />
                    )
                })}
            </Switch>
            <GlobalDebug {...props} />
        </div>
    )
}

interface AppState extends ConfigurationCascadeProps, ExtensionsProps, CXPEnvironmentProps, CXPControllerProps {
    error?: Error
    user?: GQL.IUser | null

    /**
     * Whether the light theme is enabled or not
     */
    isLightTheme: boolean

    /**
     * The current search query in the navbar.
     */
    navbarSearchQuery: string

    /** Whether the help popover is shown. */
    showHelpPopover: boolean

    /** Whether the history popover is shown. */
    showHistoryPopover: boolean
}

const LIGHT_THEME_LOCAL_STORAGE_KEY = 'light-theme'

/** A fallback configuration subject that can be constructed synchronously at initialization time. */
const SITE_SUBJECT_NO_ADMIN: Pick<GQL.IConfigurationSubject, 'id' | 'viewerCanAdminister'> = {
    id: window.context.siteGQLID,
    viewerCanAdminister: false,
}

/**
 * The root component
 */
class App extends React.Component<{}, AppState> {
    constructor(props: {}) {
        super(props)
        const extensions = createExtensionsContextController()
        this.state = {
            isLightTheme: localStorage.getItem(LIGHT_THEME_LOCAL_STORAGE_KEY) !== 'false',
            navbarSearchQuery: '',
            showHelpPopover: false,
            showHistoryPopover: false,
            configurationCascade: { subjects: null, merged: null },
            extensions,
            cxpEnvironment: CXP_EMPTY_ENVIRONMENT,
            cxpController: createCXPController(extensions.context, createMessageTransports),
        }
    }

    private subscriptions = new Subscription()

    public componentDidMount(): void {
        document.body.classList.add('theme')
        this.subscriptions.add(
            currentUser.subscribe(user => this.setState({ user }), error => this.setState({ user: null }))
        )

        this.subscriptions.add(this.state.cxpController)

        this.subscriptions.add(
            this.state.extensions.context.configurationCascade.subscribe(
                v => this.onConfigurationCascadeChange(v),
                err => console.error(err)
            )
        )

        // Keep CXP controller's extensions up-to-date.
        //
        // TODO(sqs): handle loading and errors
        this.subscriptions.add(
            this.state.extensions.viewerConfiguredExtensions.subscribe(
                extensions => this.onViewerConfiguredExtensionsChange(extensions),
                err => console.error(err)
            )
        )
    }

    public componentWillUnmount(): void {
        this.subscriptions.unsubscribe()
        document.body.classList.remove('theme')
        document.body.classList.remove('theme-light')
        document.body.classList.remove('theme-dark')
    }

    public componentDidUpdate(): void {
        localStorage.setItem(LIGHT_THEME_LOCAL_STORAGE_KEY, this.state.isLightTheme + '')
        document.body.classList.toggle('theme-light', this.state.isLightTheme)
        document.body.classList.toggle('theme-dark', !this.state.isLightTheme)
    }

    public render(): React.ReactFragment | null {
        if (this.state.error) {
            return <HeroPage icon={ErrorIcon} title={'Something happened'} subtitle={this.state.error.message} />
        }

        if (window.pageError && window.pageError.statusCode !== 404) {
            const statusCode = window.pageError.statusCode
            const statusText = window.pageError.statusText
            const errorMessage = window.pageError.error
            const errorID = window.pageError.errorID

            let subtitle: JSX.Element | undefined
            if (errorID) {
                subtitle = <FeedbackText headerText="Sorry, there's been a problem." />
            }
            if (errorMessage) {
                subtitle = (
                    <div className="app__error">
                        {subtitle}
                        {subtitle && <hr />}
                        <pre>{errorMessage}</pre>
                    </div>
                )
            } else {
                subtitle = <div className="app__error">{subtitle}</div>
            }
            return <HeroPage icon={ServerIcon} title={`${statusCode}: ${statusText}`} subtitle={subtitle} />
        }

        if (this.state.user === undefined) {
            return null
        }

        return [
            <BrowserRouter key={0}>
                <Route path="/" render={this.renderLayout} />
            </BrowserRouter>,
            <Tooltip key={1} />,
            USE_PLATFORM ? <Notifications key={2} cxpController={this.state.cxpController} /> : null,
        ]
    }

    private renderLayout = (props: RouteComponentProps<any>) => {
        let viewerSubject: LayoutProps['viewerSubject']
        if (this.state.user) {
            viewerSubject = this.state.user
        } else if (
            this.state.configurationCascade &&
            !isErrorLike(this.state.configurationCascade) &&
            this.state.configurationCascade.subjects &&
            !isErrorLike(this.state.configurationCascade.subjects) &&
            this.state.configurationCascade.subjects.length > 0
        ) {
            viewerSubject = this.state.configurationCascade.subjects[0].subject
        } else {
            viewerSubject = SITE_SUBJECT_NO_ADMIN
        }

        return (
            <Layout
                {...props}
                /* Checked for undefined in render() above */
                user={this.state.user as GQL.IUser | null}
                viewerSubject={viewerSubject}
                isLightTheme={this.state.isLightTheme}
                onThemeChange={this.onThemeChange}
                navbarSearchQuery={this.state.navbarSearchQuery}
                onNavbarQueryChange={this.onNavbarQueryChange}
                showHelpPopover={this.state.showHelpPopover}
                showHistoryPopover={this.state.showHistoryPopover}
                onHelpPopoverToggle={this.onHelpPopoverToggle}
                onHistoryPopoverToggle={this.onHistoryPopoverToggle}
                configurationCascade={this.state.configurationCascade}
                extensions={this.state.extensions}
                cxpEnvironment={this.state.cxpEnvironment}
                cxpOnComponentChange={this.cxpOnComponentChange}
                cxpOnRootChange={this.cxpOnRootChange}
                cxpController={this.state.cxpController}
            />
        )
    }

    private onThemeChange = () => {
        this.setState(
            state => ({ isLightTheme: !state.isLightTheme }),
            () => {
                eventLogger.log(this.state.isLightTheme ? 'LightThemeClicked' : 'DarkThemeClicked')
            }
        )
    }

    private onNavbarQueryChange = (navbarSearchQuery: string) => {
        this.setState({ navbarSearchQuery })
    }

    private onHelpPopoverToggle = (visible?: boolean): void => {
        eventLogger.log('HelpPopoverToggled')
        this.setState(prevState => ({
            // If visible is any non-boolean type (e.g., MouseEvent), treat it as undefined. This lets callers use
            // onHelpPopoverToggle directly in an event handler without wrapping it in an another function.
            showHelpPopover: visible !== true && visible !== false ? !prevState.showHelpPopover : visible,
        }))
    }

    private onHistoryPopoverToggle = (visible?: boolean): void => {
        eventLogger.log('HistoryPopoverToggled')
        this.setState(prevState => ({
            showHistoryPopover: visible !== true && visible !== false ? !prevState.showHistoryPopover : visible,
        }))
    }

    private onConfigurationCascadeChange(
        configurationCascade: ConfigurationCascadeOrError<ConfigurationSubject, Settings>
    ): void {
        this.setState(
            prevState => {
                const update: Pick<AppState, 'configurationCascade' | 'cxpEnvironment'> = {
                    configurationCascade,
                    cxpEnvironment: prevState.cxpEnvironment,
                }
                if (
                    configurationCascade.subjects !== null &&
                    !isErrorLike(configurationCascade.subjects) &&
                    configurationCascade.merged !== null &&
                    !isErrorLike(configurationCascade.merged)
                ) {
                    // Only update CXP environment configuration if the configuration was successfully parsed.
                    //
                    // TODO(sqs): Think through how this error should be handled.
                    update.cxpEnvironment = {
                        ...prevState.cxpEnvironment,
                        configuration: {
                            subjects: configurationCascade.subjects.filter(
                                (subject): subject is ConfiguredSubject<ConfigurationSubject, Settings> =>
                                    subject.settings !== null && !isErrorLike(subject.settings)
                            ),
                            merged: configurationCascade.merged,
                        },
                    }
                }
                return update
            },
            () => this.state.cxpController.setEnvironment(this.state.cxpEnvironment)
        )
    }

    private onViewerConfiguredExtensionsChange(viewerConfiguredExtensions: ConfiguredExtension[]): void {
        this.setState(
            prevState => ({
                cxpEnvironment: {
                    ...prevState.cxpEnvironment,
                    extensions: viewerConfiguredExtensions,
                },
            }),
            () => this.state.cxpController.setEnvironment(this.state.cxpEnvironment)
        )
    }

    private cxpOnComponentChange = (component: CXPComponent | null): void => {
        this.setState(
            prevState => ({ cxpEnvironment: { ...prevState.cxpEnvironment, component } }),
            () => this.state.cxpController.setEnvironment(this.state.cxpEnvironment)
        )
    }

    private cxpOnRootChange = (root: URI | null): void => {
        this.setState(
            prevState => ({ cxpEnvironment: { ...prevState.cxpEnvironment, root } }),
            () => this.state.cxpController.setEnvironment(this.state.cxpEnvironment)
        )
    }
}

window.addEventListener('DOMContentLoaded', () => {
    render(<App />, document.querySelector('#root'))
})

updateUserSessionStores()
