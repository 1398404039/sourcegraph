import ArrowCollapseVerticalIcon from '@sourcegraph/icons/lib/ArrowCollapseVertical'
import ArrowExpandVerticalIcon from '@sourcegraph/icons/lib/ArrowExpandVertical'
import CalculatorIcon from '@sourcegraph/icons/lib/Calculator'
import CheckmarkIcon from '@sourcegraph/icons/lib/Checkmark'
import DirectionalSign from '@sourcegraph/icons/lib/DirectionalSign'
import DownloadIcon from '@sourcegraph/icons/lib/Download'
import HourglassIcon from '@sourcegraph/icons/lib/Hourglass'
import SaveIcon from '@sourcegraph/icons/lib/Save'
import * as React from 'react'
import * as GQL from '../../backend/graphqlschema'
import { ServerBanner } from '../../marketing/ServerBanner'
import { showDotComMarketing } from '../../util/features'
import { pluralize } from '../../util/strings'

const showMissingReposEnabled = window.context.showMissingReposEnabled || localStorage.getItem('showMissingRepos')

interface SearchResultsInfoBarProps {
    /** The logged-in user or null */
    user: GQL.IUser | null

    /** The loaded search results and metadata */
    results: GQL.ISearchResults

    // Expand all feature
    allExpanded: boolean
    onExpandAllResultsToggle: () => void

    // Saved queries
    onDidCreateSavedQuery: () => void
    onSaveQueryClick: () => void
    didSave: boolean
}

/**
 * The info bar shown over the search results list that displays metadata
 * and a few actions like expand all and save query
 */
export const SearchResultsInfoBar: React.StatelessComponent<SearchResultsInfoBarProps> = props => (
    <div className="search-results-info-bar">
        {(props.results.timedout.length > 0 ||
            props.results.cloning.length > 0 ||
            props.results.results.length > 0 ||
            (showMissingReposEnabled && props.results.missing.length > 0)) && (
            <small className="search-results-info-bar__row">
                <div className="search-results-info-bar__row-left">
                    {/* Time stats */}
                    {
                        <div className="search-results-info-bar__notice e2e-search-results-stats">
                            <span>
                                <CalculatorIcon className="icon-inline" /> {props.results.approximateResultCount}{' '}
                                {pluralize('result', props.results.resultCount)} in{' '}
                                {(props.results.elapsedMilliseconds / 1000).toFixed(2)} seconds
                                {props.results.indexUnavailable && ' (index unavailable)'}
                            </span>
                        </div>
                    }
                    {/* Missing repos */}
                    {showMissingReposEnabled &&
                        props.results.missing.length > 0 && (
                            <div
                                className="search-results-info-bar__notice"
                                data-tooltip={props.results.missing.join('\n')}
                            >
                                <span>
                                    <DirectionalSign className="icon-inline" /> {props.results.missing.length}{' '}
                                    {pluralize('repository', props.results.missing.length, 'repositories')} not found
                                </span>
                            </div>
                        )}
                    {/* Timed out repos */}
                    {props.results.timedout.length > 0 && (
                        <div
                            className="search-results-info-bar__notice"
                            data-tooltip={props.results.timedout.join('\n')}
                        >
                            <span>
                                <HourglassIcon className="icon-inline" /> {props.results.timedout.length}{' '}
                                {pluralize('repository', props.results.timedout.length, 'repositories')} timed out
                                (reload to try again, or specify a longer "timeout:" in your query)
                            </span>
                        </div>
                    )}
                    {/* Cloning repos */}
                    {props.results.cloning.length > 0 && (
                        <div
                            className="search-results-info-bar__notice"
                            data-tooltip={props.results.cloning.join('\n')}
                        >
                            <span>
                                <DownloadIcon className="icon-inline" /> {props.results.cloning.length}{' '}
                                {pluralize('repository', props.results.cloning.length, 'repositories')} cloning (reload
                                to try again)
                            </span>
                        </div>
                    )}
                </div>
                <div className="search-results-info-bar__row-right">
                    {/* Expand all feature */}
                    <button onClick={props.onExpandAllResultsToggle} className="btn btn-link">
                        {props.allExpanded ? (
                            <>
                                <ArrowCollapseVerticalIcon className="icon-inline" data-tooltip="Collapse" /> Collapse
                                all
                            </>
                        ) : (
                            <>
                                <ArrowExpandVerticalIcon className="icon-inline" data-tooltip="Expand" /> Expand all
                            </>
                        )}
                    </button>
                    {/* Saved Queries */}
                    {props.user && (
                        <button onClick={props.onSaveQueryClick} className="btn btn-link" disabled={props.didSave}>
                            {props.didSave ? (
                                <>
                                    <CheckmarkIcon className="icon-inline" /> Query saved
                                </>
                            ) : (
                                <>
                                    <SaveIcon className="icon-inline" /> Save this search query
                                </>
                            )}
                        </button>
                    )}
                </div>
            </small>
        )}
        {!props.results.alert && showDotComMarketing && <ServerBanner />}
    </div>
)
