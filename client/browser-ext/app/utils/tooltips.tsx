import * as _ from "lodash";
import * as marked from "marked";
import { style } from "typestyle";
import { eventLogger } from "../utils/context";

// tslint:disable-next-line
const truncate = require("html-truncate");

export type TooltipData = { title: string, doc?: string } | null;

const tooltipClassName = style({
	backgroundColor: "#2D2D30",
	maxWidth: "500px",
	maxHeight: "250px",
	border: "solid 1px #555",
	borderRadius: "7px",
	fontFamily: `"Helvetica Neue", Helvetica, Arial, sans-serif`,
	color: "rgba(199, 216, 229, 1)",
	fontSize: "14px",
	zIndex: 100,
	position: "absolute",
	overflow: "auto",
	padding: "10px 10px",
	boxShadow: "2px 2px 1px #888888",
});

const tooltipTitleStyle = style({
	fontFamily: `Menlo, Monaco, Consolas, "Courier New", monospace`,
	wordWrap: "break-all",
});

const tooltipDocStyle = style({
	marginTop: "5px",
	paddingTop: "10px",
	fontFamily: `"Helvetica Neue", Helvetica, Arial, sans-serif`,
	wordWrap: "break-all",
});

let tooltip;
let loadingTooltip;

/**
 * createTooltips initializes the DOM elements used for the hover
 * tooltip and "Loading..." text indicator, adding the former
 * to the DOM (but hidden). It is idempotent.
 */
export function createTooltips(): void {
	if (tooltip || loadingTooltip) {
		return; // idempotence
	}

	tooltip = document.createElement("DIV");
	tooltip.className = tooltipClassName;
	tooltip.classList.add("sg-tooltip");
	tooltip.style.visibility = "hidden";
	document.body.appendChild(tooltip);

	loadingTooltip = document.createElement("DIV");
	loadingTooltip.appendChild(document.createTextNode("Loading..."));
};

let activeTarget: HTMLElement | null = null;
let hoverEventProps: Object | null = null;

/**
 * setContext registers the active target (element) being moused over, as well
 * as properties to send to the event logger on when the tooltip is shown.
 */
export function setContext(target: HTMLElement | null, loggingStruct: Object | null): void {
	activeTarget = target;
	hoverEventProps = loggingStruct;
}

/**
 * clearContext removes all registered tooltip state and hides the tooltip.
 */
export function clearContext(): void {
	setContext(null, null);
	setTooltip(null, null);
	hideTooltip();
}

let currentTooltipText: string | null = null;
let currentTooltipDoc: string | null = null;
let isLoading = false; // whether the tooltip should show "Loading..." text

let loadingTimer: NodeJS.Timer; // a handle to a timeout which sets the "Loading..." text indicator

/**
 * clearLoading clears the "Loading..." tooltip, as well as any timeout
 * which would show the loading text.
 */
function clearLoading(): void {
	if (loadingTimer) {
		clearTimeout(loadingTimer);
	}
	isLoading = false;
}

/**
 * queueLoading shows the "Loading..." tooltip after 0.5 seconds.
 */
export function queueLoading(): void {
	clearLoading();
	loadingTimer = setTimeout(() => {
		isLoading = true;
		updateTooltip(activeTarget);
	}, 500);
}

/**
 * setTooltip shows the provided tooltip text (or hides the tooltip, if a null
 * argument is provided). It overrides the "Loading..." tooltip.
 */
export function setTooltip(data: TooltipData, target: HTMLElement | null): void {
	if (target !== activeTarget) {
		// setTooltip is called asynchronously after a fetch; only update the tooltip
		// if the currently set active target matches the target argument
		return;
	}
	clearLoading();

	if (!data) {
		currentTooltipText = null;
	} else {
		currentTooltipText = data.title;
		currentTooltipDoc = data.doc || null;
	}
	updateTooltip(target);
}

/**
 * hideTooltip makes the tooltip on the DOM invisible.
 */
export function hideTooltip(): void {
	if (!tooltip) {
		return;
	}

	while (tooltip.firstChild) {
		tooltip.removeChild(tooltip.firstChild);
	}
	tooltip.style.visibility = "hidden"; // prevent black dot of empty content
}

/**
 * _updateTooltip displays the appropriate tooltip given current state (and may hide
 * the tooltip if no text is available).
 */
function _updateTooltip(target: HTMLElement | null): void {
	hideTooltip(); // hide before updating tooltip text

	if (!target) {
		// no target to show hover for; tooltip is hidden
		return;
	}

	if (!isLoading) {
		if (!currentTooltipText) {
			// no tooltip text for hovered token; tooltip is hidden
			return;
		}

		target.style.cursor = "pointer";
		if (!target.className.includes("sg-clickable")) {
			target.className = `${target.className} sg-clickable`;
		}

		const tooltipText = document.createElement("DIV");
		tooltipText.className = tooltipTitleStyle;
		tooltipText.appendChild(document.createTextNode(currentTooltipText));
		tooltip.appendChild(tooltipText);

		if (currentTooltipDoc) {
			const tooltipDoc = document.createElement("DIV");
			tooltipDoc.className = tooltipDocStyle;
			tooltipDoc.innerHTML = truncate(marked(currentTooltipDoc, { gfm: true, breaks: true, sanitize: true }), 300);
			tooltip.appendChild(tooltipDoc);
		}

		// only log when displaying a real tooltip (not a loading indicator)
		eventLogger.logHover(Object.assign({}, hoverEventProps || undefined));
	} else {
		tooltip.appendChild(loadingTooltip);
	}

	// Anchor it horizontally, prior to rendering to account for wrapping
	// changes to vertical height if the tooltip is at the edge of the viewport.
	const targetBound = target.getBoundingClientRect();
	tooltip.style.left = (targetBound.left + window.scrollX) + "px";

	// Anchor the tooltip vertically.
	const tooltipBound = tooltip.getBoundingClientRect();
	tooltip.style.top = (targetBound.top - (tooltipBound.height + 5) + window.scrollY) + "px";

	// Make it all visible to the user.
	tooltip.style.visibility = "visible";
}

const updateTooltip = _.throttle(_updateTooltip, 50, { leading: true, trailing: true }); // prevent tooltip flashes as cursor moves quickly across the page
