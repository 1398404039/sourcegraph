import React from 'react'
import { Link } from 'react-router-dom'
import { CodeExcerpt } from '../components/CodeExcerpt'
import { RepoFileLink } from '../components/RepoFileLink'
import { pluralize } from '../util/strings'
import { toPrettyBlobURL } from '../util/url'
import { CodeExcerpt2 } from './CodeExcerpt2'
import { ResultContainer } from './ResultContainer'

export interface IFileMatch {
    resource: string
    lineMatches: ILineMatch[]
    limitHit?: boolean
}

export interface ILineMatch {
    preview: string
    lineNumber: number
    offsetAndLengths: number[][]
    limitHit?: boolean
}

interface Props {
    /**
     * The file match search result.
     */
    result: IFileMatch

    /**
     * The icon to show left to the title.
     */
    icon: React.ComponentType<{ className: string }>

    /**
     * Called when the file's search result is selected.
     */
    onSelect: () => void

    /**
     * Whether this file should be rendered as expanded.
     */
    expanded: boolean

    /**
     * Whether or not to show all matches for this file, or only a subset.
     */
    showAllMatches: boolean

    isLightTheme: boolean
}

const subsetMatches = 2

// Dev flag for disabling syntax highlighting on search results pages.
const NO_SEARCH_HIGHLIGHTING = localStorage.getItem('noSearchHighlighting') !== null

export const FileMatch: React.StatelessComponent<Props> = (props: Props) => {
    const parsed = new URL(props.result.resource)
    const repoPath = parsed.hostname + parsed.pathname
    const rev = parsed.search.substr('?'.length)
    const filePath = parsed.hash.substr('#'.length)
    const items = props.result.lineMatches.map(match => ({
        highlightRanges: match.offsetAndLengths.map(offsetAndLength => ({
            start: offsetAndLength[0],
            highlightLength: offsetAndLength[1],
        })),
        preview: match.preview,
        line: match.lineNumber,
        uri: props.result.resource,
        repoURI: repoPath,
    }))

    const title = <RepoFileLink repoPath={repoPath} filePath={filePath} />

    const getChildren = (allMatches: boolean) => {
        const showItems = items
            .sort((a, b) => {
                if (a.line < b.line) {
                    return -1
                }
                if (a.line === b.line) {
                    if (a.highlightRanges[0].start < b.highlightRanges[0].start) {
                        return -1
                    }
                    if (a.highlightRanges[0].start === b.highlightRanges[0].start) {
                        return 0
                    }
                    return 1
                }
                return 1
            })
            .filter((item, i) => allMatches || i < subsetMatches)

        if (NO_SEARCH_HIGHLIGHTING) {
            return (
                <CodeExcerpt2
                    urlWithoutPosition={toPrettyBlobURL({ repoPath, rev, filePath })}
                    items={showItems}
                    onSelect={props.onSelect}
                />
            )
        }

        return (
            <div className="file-match__list">
                {showItems.map((item, i) => {
                    const uri = new URL(item.uri)
                    const position = { line: item.line + 1, character: item.highlightRanges[0].start + 1 }
                    return (
                        <Link
                            to={toPrettyBlobURL({
                                repoPath: uri.hostname + uri.pathname,
                                rev,
                                filePath: uri.hash.substr('#'.length),
                                position,
                            })}
                            key={i}
                            className="file-match__item file-match__item-clickable"
                            onClick={props.onSelect}
                        >
                            <CodeExcerpt
                                repoPath={repoPath}
                                commitID={rev}
                                filePath={filePath}
                                previewWindowExtraLines={1}
                                highlightRanges={item.highlightRanges}
                                line={item.line}
                                isLightTheme={props.isLightTheme}
                            />
                        </Link>
                    )
                })}
            </div>
        )
    }

    if (props.showAllMatches) {
        return (
            <ResultContainer
                collapsible={true}
                defaultExpanded={props.expanded}
                icon={props.icon}
                title={title}
                expandedChildren={getChildren(true)}
            />
        )
    } else {
        return (
            <ResultContainer
                collapsible={items.length > subsetMatches}
                defaultExpanded={props.expanded}
                icon={props.icon}
                title={title}
                collapsedChildren={getChildren(false)}
                expandedChildren={getChildren(true)}
                collapseLabel={`Hide ${items.length - subsetMatches} matches`}
                expandLabel={`Show ${items.length - subsetMatches} more ${pluralize(
                    'match',
                    items.length - subsetMatches,
                    'matches'
                )}`}
            />
        )
    }
}
