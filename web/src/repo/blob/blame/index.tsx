import { BehaviorSubject } from 'rxjs/BehaviorSubject'
import { Observable } from 'rxjs/Observable'
import { interval } from 'rxjs/observable/interval'
import { merge } from 'rxjs/observable/merge'
import { map } from 'rxjs/operators/map'
import { switchMap } from 'rxjs/operators/switchMap'
import { take } from 'rxjs/operators/take'
import { takeUntil } from 'rxjs/operators/takeUntil'
import { openFromJS, toCommitURL } from '../../../util/url'
import { AbsoluteRepoFilePosition, AbsoluteRepoFileRange } from '../../index'
import { fetchBlameFile } from './backend'
import { clearLineBlameContent, setLineBlame } from './dom'

export interface BlameData {
    ctx: AbsoluteRepoFileRange
    hunks: GQL.IHunk[]
    loading: boolean
}

/**
 * Measures the width of the given text string in pixels, using the given font.
 * @param text the literal text to measure.
 * @param font the font string, e.g. '12px Menlo'
 */
function measureTextWidth(text: string, font: string): number {
    const tmp = document.createElement('canvas').getContext('2d')
    tmp!.font = font
    return tmp!.measureText(text).width
}

/**
 * A stream of events to trigger showing blame data on a line.
 * We subscribe to the stream to fetch data and update the DOM.
 * The switch map prevents race conditions; as new lines are clicked,
 * prior fetches will be unsubscribed from and so the DOM will only be updated
 * by data fetched for the most recent event. Use a BehaviorSubject b/c
 * maybeOpenCommit() needs to look at the current value.
 */
const blameEvents = new BehaviorSubject<AbsoluteRepoFileRange | null>(null)
blameEvents
    .pipe(
        switchMap(ctx => {
            if (!ctx) {
                return []
            }
            const fetch: Observable<BlameData> = fetchBlameFile({
                ...ctx,
                position: { line: ctx.range.start.line, character: 0 },
            } as AbsoluteRepoFilePosition).pipe(map(hunks => ({ ctx, loading: false, hunks: hunks || [] })))
            // show loading data after 250ms if the fetch has not resolved
            const loading: Observable<BlameData> = interval(250).pipe(
                take(1),
                takeUntil(fetch),
                map(() => ({ ctx, loading: true, hunks: [] }))
            )
            return merge(loading, fetch)
        })
    )
    .subscribe(setLineBlame)

/**
 * opens the commit if the userTriggered event exists and was inside the blame text shown
 * previously on the same line.
 * @param ctx the blame context
 * @param userTriggered the click event
 */
function maybeOpenCommit(ctx: AbsoluteRepoFileRange, clickEvent?: MouseEvent): void {
    if (!clickEvent) {
        return
    }
    const prevCtx = blameEvents.getValue()
    const currentlyBlamed = document.querySelector('.blob td.code>.blame')
    if (!prevCtx || prevCtx.range.start.line !== ctx.range.start.line || !currentlyBlamed) {
        return // Not clicking on a line with blame info already showing.
    }
    const rev = currentlyBlamed.getAttribute('data-blame-rev')
    if (!rev) {
        return // e.g. if blame info failed to load or is currently loading
    }

    /**
     * Blame information is rendered in a ::before pseudo-element to avoid it being copied
     * when trying to copy code. This spared us from having to do absolute positioning of
     * the blame text onto the line itself as a non-child element of the blob view.
     *
     * However, the pseudo-element makes detecting clicks on the blame information (to view
     * the commit) hard because psuedo-elements have no DOM representation. We use a hack
     * here: We know the user clicked on the line with blame information, so we measure the
     * width of the blame text and if the mouse click was in its range then they clicked on
     * the blame text.
     *
     * TODO(future): Let's make blame text absolutely positioned on top of the line (not a
     * child of blob view), and turn all of this into a React component.
     */
    const x = clickEvent.clientX
    const blameTextStart = currentlyBlamed.getBoundingClientRect().right
    const blameTextEnd = blameTextStart + measureTextWidth(currentlyBlamed.getAttribute('data-blame')!, '12px Menlo')
    if (x < blameTextStart || x > blameTextEnd) {
        return // Not clicking on blame text
    }

    openFromJS(toCommitURL({ repoPath: ctx.repoPath, commitID: rev }), clickEvent)
}

export function triggerBlame(ctx: AbsoluteRepoFileRange, clickEvent?: MouseEvent): void {
    maybeOpenCommit(ctx, clickEvent) // important: must come before updating subject
    clearLineBlameContent()
    blameEvents.next(ctx)
}
