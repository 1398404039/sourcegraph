import React from 'react'
import VisibilitySensor from 'react-visibility-sensor'
import { Subscription } from 'rxjs/Subscription'
import { AbsoluteRepoFile } from '../repo'
import { fetchHighlightedFileLines } from '../repo/backend'
import { colorTheme, getColorTheme } from '../settings/theme'
import { highlightNode } from '../util/dom'

interface Props extends AbsoluteRepoFile {
    // How many extra lines to show in the excerpt before/after the ref.
    previewWindowExtraLines?: number
    line: number
    highlightRanges: HighlightRange[]
}

interface HighlightRange {
    /**
     * The 0-based character offset to start highlighting at
     */
    start: number
    /**
     * The number of characters to highlight
     */
    highlightLength: number
}

interface State {
    blobLines?: string[]
    isLightTheme: boolean
}

export class CodeExcerpt extends React.PureComponent<Props, State> {
    private tableContainerElement: HTMLElement | null = null
    private isVisible = false
    private subscriptions = new Subscription()

    constructor(props: Props) {
        super(props)
        this.state = { isLightTheme: getColorTheme() === 'light' }
    }

    public componentDidMount(): void {
        this.subscriptions.add(
            colorTheme.subscribe(theme =>
                this.setState({ isLightTheme: theme === 'light' }, () => {
                    if (this.isVisible) {
                        this.fetchContents(this.props)
                    }
                })
            )
        )
    }

    public componentWillReceiveProps(nextProps: Props): void {
        if (this.isVisible) {
            this.fetchContents(nextProps)
        }
    }

    public componentDidUpdate(prevProps: Props, prevState: State): void {
        if (this.tableContainerElement) {
            const rows = this.tableContainerElement.querySelectorAll('table tr')
            for (const row of rows) {
                const line = row.firstChild as HTMLTableDataCellElement
                const code = row.lastChild as HTMLTableDataCellElement
                if (line.getAttribute('data-line') === '' + (this.props.line + 1)) {
                    for (const range of this.props.highlightRanges) {
                        highlightNode(code, range.start, range.highlightLength)
                    }
                }
            }
        }
    }

    public componentWillUnmount(): void {
        this.subscriptions.unsubscribe()
    }

    public getPreviewWindowLines(): number[] {
        const targetLine = this.props.line
        let res = [targetLine]
        for (
            let i = targetLine - this.props.previewWindowExtraLines!;
            i < targetLine + this.props.previewWindowExtraLines! + 1;
            ++i
        ) {
            if (i > 0 && i < targetLine) {
                res = [i].concat(res)
            }
            if (this.state.blobLines) {
                if (i < this.state.blobLines!.length && i > targetLine) {
                    res = res.concat([i])
                }
            } else {
                if (i > targetLine) {
                    res = res.concat([i])
                }
            }
        }
        return res
    }

    public onChangeVisibility = (isVisible: boolean): void => {
        this.isVisible = isVisible
        if (isVisible) {
            this.fetchContents(this.props)
        }
    }

    public render(): JSX.Element | null {
        return (
            <VisibilitySensor onChange={this.onChangeVisibility} partialVisibility={true}>
                <code className="code-excerpt">
                    {this.state.blobLines && (
                        <div
                            ref={this.setTableContainerElement}
                            dangerouslySetInnerHTML={{ __html: this.makeTableHTML() }}
                        />
                    )}
                    {!this.state.blobLines && (
                        <table>
                            <tbody>
                                {this.getPreviewWindowLines().map(i => (
                                    <tr key={i}>
                                        <td className="line">{i + 1}</td>
                                        {/* create empty space to fill viewport (as if the blob content were already fetched, otherwise we'll overfetch) */}
                                        <td className="code"> </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    )}
                </code>
            </VisibilitySensor>
        )
    }

    private setTableContainerElement = (ref: HTMLElement | null) => {
        this.tableContainerElement = ref
    }

    private fetchContents(props: Props): void {
        fetchHighlightedFileLines({
            repoPath: props.repoPath,
            commitID: props.commitID,
            filePath: props.filePath,
            disableTimeout: true,
            isLightTheme: this.state.isLightTheme,
        }).subscribe(
            lines => {
                this.setState({ blobLines: lines })
            },
            err => {
                console.error('failed to fetch blob content', err)
            }
        )
    }

    private makeTableHTML(): string {
        const start = Math.max(0, this.props.line - (this.props.previewWindowExtraLines || 0))
        const end = this.props.line + (this.props.previewWindowExtraLines || 0) + 1
        const lineRange = this.state.blobLines!.slice(start, end)
        return '<table>' + lineRange.join('') + '</table>'
    }
}
