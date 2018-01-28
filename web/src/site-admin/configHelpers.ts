import { setProperty } from '@sqs/jsonc-parser/lib/edit'
import { Edit, FormattingOptions } from '@sqs/jsonc-parser/lib/format'
import { parse } from '@sqs/jsonc-parser/lib/main'
import {
    GitHubConnection,
    OpenIdConnectAuthProvider,
    Repository,
    SamlAuthProvider,
    SiteConfiguration,
} from '../schema/site.schema'

/**
 * A helper function that modifies site configuration to configure specific
 * common things, such as syncing GitHub repositories.
 */
type ConfigHelper = (configJSON: string) => { edits: Edit[]; selectText?: string }

const defaultFormattingOptions: FormattingOptions = {
    eol: '\n',
    insertSpaces: true,
    tabSize: 2,
}

const addGitHubDotCom: ConfigHelper = config => {
    const tokenPlaceholder = '<personal access token with repo scope (https://github.com/settings/tokens/new)>'
    const value: GitHubConnection = {
        token: tokenPlaceholder,
        url: 'https://github.com',
    }
    const edits = setProperty(config, ['github', -1], value, defaultFormattingOptions)
    return { edits, selectText: tokenPlaceholder }
}

const addGitHubEnterprise: ConfigHelper = config => {
    const tokenPlaceholder = '<personal access token with repo scope>'
    const value: GitHubConnection = {
        token: tokenPlaceholder,
        url: 'https://github-enterprise-hostname.example.com',
    }
    const edits = setProperty(config, ['github', -1], value, defaultFormattingOptions)
    return { edits, selectText: tokenPlaceholder }
}

const addOtherRepository: ConfigHelper = config => {
    const urlPlaceholder = '<git clone URL>'
    const value: Repository = {
        url: urlPlaceholder,
        path: '<desired name of repository on Sourcegraph (example: my/repo)>',
    }
    const edits = setProperty(config, ['repos.list', -1], value, defaultFormattingOptions)
    return { edits, selectText: urlPlaceholder }
}

const addSSOViaGSuite: ConfigHelper = config => {
    const value: OpenIdConnectAuthProvider = {
        issuer: 'https://accounts.google.com',
        clientID: '<see documentation: https://developers.google.com/identity/protocols/OpenIDConnect#getcredentials>',
        clientSecret: '<see same documentation as clientID>',
        requireEmailDomain: "<your company's email domain (example: mycompany.com)>",
    }
    return {
        edits: [
            ...setProperty(config, ['auth.provider'], 'openidconnect', defaultFormattingOptions),
            ...setProperty(config, ['auth.openIDConnect'], value, defaultFormattingOptions),
        ],
        selectText: '"auth.openIDConnect": {',
    }
}

const addSSOViaSAML: ConfigHelper = config => {
    const value: SamlAuthProvider = {
        identityProviderMetadataURL: '<see https://about.sourcegraph.com/docs/server/config/authentication#saml>',
        serviceProviderCertificate: '<see https://about.sourcegraph.com/docs/server/config/authentication#saml>',
        serviceProviderPrivateKey: '<see https://about.sourcegraph.com/docs/server/config/authentication#saml>',
    }
    return {
        edits: [
            ...setProperty(config, ['auth.provider'], 'saml', defaultFormattingOptions),
            ...setProperty(config, ['auth.saml'], value, defaultFormattingOptions),
        ],
        selectText: '"auth.saml": {',
    }
}

const addSearchScopeToSettings: ConfigHelper = config => {
    const value: { name: string; value: string } = {
        name: '<name>',
        value: '<partial query string that will be inserted when the scope is selected>',
    }
    const edits = setProperty(config, ['search.scopes', -1], value, defaultFormattingOptions)
    return { edits, selectText: '<name>' }
}

const addOpenSearch: ConfigHelper = config => {
    const value: { searchUrl: string } = {
        searchUrl: '<URL with {searchTerms} in place of query (example: http://localhost:7080/search?q={searchTerms})',
    }
    const edits = [...setProperty(config, ['openSearch'], value, defaultFormattingOptions)]
    return { edits, selectText: value.searchUrl }
}

export interface EditorAction {
    id: string
    label: string
    run: ConfigHelper
}

export const settingsActions: EditorAction[] = [
    { id: 'sourcegraph.settings.searchScopes', label: 'Add search scope', run: addSearchScopeToSettings },
]

export const siteConfigActions: EditorAction[] = [
    { id: 'sourcegraph.site.githubDotCom', label: 'Add GitHub.com repositories', run: addGitHubDotCom },
    {
        id: 'sourcegraph.site.githubEnterprise',
        label: 'Add GitHub Enterprise repositories',
        run: addGitHubEnterprise,
    },
    { id: 'sourcegraph.site.otherRepository', label: 'Add other repository', run: addOtherRepository },
    { id: 'sourcegraph.site.ssoViaGSuite', label: 'Use SSO via Google (G Suite)', run: addSSOViaGSuite },
    { id: 'sourcegraph.site.ssoViaSAML', label: 'Use SSO via SAML', run: addSSOViaSAML },
    { id: 'sourcegraph.site.openSearch', label: 'Add address bar search', run: addOpenSearch },
]

/** Parses the JSON site configuration provided. */
export function parseSiteConfiguration(text: string): SiteConfiguration {
    const o = parse(text, [], { allowTrailingComma: true, disallowComments: false })
    return o as SiteConfiguration
}

/** Parses out the 'disableTelemetry' key from the JSON site config and returns the inverse. */
export function getTelemetryEnabled(text: string): boolean {
    return !parseSiteConfiguration(text).disableTelemetry
}
