// @flow weak

import React from "react";
import {Link} from "react-router";

import {annotate} from "sourcegraph/blob/Annotations";
import classNames from "classnames";
import Component from "sourcegraph/Component";
import Dispatcher from "sourcegraph/Dispatcher";
import {urlToBlob} from "sourcegraph/blob/routes";
import * as BlobActions from "sourcegraph/blob/BlobActions";
import * as DefActions from "sourcegraph/def/DefActions";
import s from "sourcegraph/blob/styles/Blob.css";
import "sourcegraph/components/styles/code.css";

// simpleContentsString converts [string...] (like ["a", "b", "c"]) to
// a string by joining the elements (to produce "abc", for example).
function simpleContentsString(contents) {
	if (!(contents instanceof Array)) return contents;
	if (contents.some((e) => typeof e !== "string")) return contents;
	return contents.join("");
}

class BlobLine extends Component {
	reconcileState(state, props) {
		state.repo = props.repo || null;
		state.rev = props.rev || null;
		state.path = props.path || null;

		// Update ownAnnURLs when they change.
		if (state.annotations !== props.annotations) {
			state.annotations = props.annotations;
			if (state.annotations && state.annotations.length) {
				state.ownAnnURLs = {};
				state.annotations.forEach((ann) => {
					if (ann.URL) state.ownAnnURLs[ann.URL] = true;
					if (ann.URLs) ann.URLs.forEach((url) => state.ownAnnURLs[url] = true);
				});
			} else {
				state.ownAnnURLs = null;
			}
		}

		// Filter to improve perf.
		state.highlightedDef = state.ownAnnURLs && state.ownAnnURLs[props.highlightedDef] ? props.highlightedDef : null;
		state.highlightedDefObj = state.highlightedDef ? props.highlightedDefObj : null;
		state.activeDef = state.ownAnnURLs && state.ownAnnURLs[props.activeDef] ? props.activeDef : null;
		state.activeDefNoRev = state.ownAnnURLs && state.ownAnnURLs[props.activeDefNoRev] ? props.activeDefNoRev : null;

		state.lineNumber = props.lineNumber || null;
		state.oldLineNumber = props.oldLineNumber || null;
		state.newLineNumber = props.newLineNumber || null;
		state.startByte = props.startByte;
		state.contents = props.contents;
		state.selected = Boolean(props.selected);
		state.className = props.className || "";
	}

	_hasLink(content) {
		if (!(content instanceof Array)) return false;
		return content.some(item => {
			if (item.type === "a") {
				return true;
			}
			let props = item.props || {};
			return this._hasLink(props.children);
		});
	}

	_annotate() {
		const hasURL = (ann, url) => url && (ann.URL ? ann.URL === url : ann.URLs.includes(url));
		let i = 0;
		return annotate(this.state.contents, this.state.startByte, this.state.annotations, (ann, content) => {
			i++;
			// ensure there are no links inside content to make ReactJS happy
			// otherwise incorrect DOM is built (a > .. > a)
			if ((ann.URL || ann.URLs) && !this._hasLink(content)) {
				let isHighlighted = hasURL(ann, this.state.highlightedDef);
				return (
					<Link
						className={classNames(ann.Class, {
							[s.highlightedAnn]: isHighlighted && (!this.state.highlightedDefObj || !this.state.highlightedDefObj.Error),

							// disabledAnn is an ann that you can't click on (possibly a broken ref).
							[s.disabledAnn]: isHighlighted && (this.state.highlightedDefObj && this.state.highlightedDefObj.Error),

							[s.activeAnn]: hasURL(ann, this.state.activeDef) || hasURL(ann, this.state.activeDefNoRev),
						})}
						to={ann.URL || ann.URLs[0]}
						onMouseOver={() => Dispatcher.Stores.dispatch(new DefActions.HighlightDef(ann.URL || ann.URLs[0]))}
						onMouseOut={() => Dispatcher.Stores.dispatch(new DefActions.HighlightDef(null))}
						onClick={(ev) => {
							if (ev.altKey || ev.ctrlKey || ev.metaKey || ev.shiftKey) return;
							// TODO: implement multiple defs menu if ann.URLs.length > 0 (more important for languages other than Go)
							if (this.state.highlightedDefObj && this.state.highlightedDefObj.Error) {
								// Prevent navigating to a broken ref or not-yet-loaded def.
								ev.preventDefault();
							}
						}}
						key={i}>{simpleContentsString(content)}</Link>
				);
			}
			return <span key={i} className={ann.Class}>{simpleContentsString(content)}</span>;
		});
	}

	render() {
		let contents = this.state.annotations ? this._annotate() : this.state.contents;
		let isDiff = this.state.oldLineNumber || this.state.newLineNumber;

		return (
			<tr className={s.line}
				data-line={this.state.lineNumber}>
				{this.state.lineNumber &&
					<td className={s.lineNumberCell} onClick={(event) => {
						if (event.shiftKey) {
							event.preventDefault();
							Dispatcher.Stores.dispatch(new BlobActions.SelectLineRange(this.state.repo, this.state.rev, this.state.path, this.state.lineNumber));
							return;
						}
					}}>
						<Link className={this.state.selected ? s.selectedLineNumber : s.lineNumber}
							to={`${urlToBlob(this.state.repo, this.state.rev, this.state.path)}#L${this.state.lineNumber}`} data-line={this.state.lineNumber} />
					</td>}
				{isDiff && <td className="line-number" data-line={this.state.oldLineNumber || ""}></td>}
				{isDiff && <td className="line-number" data-line={this.state.newLineNumber || ""}></td>}

				<td className={`code ${this.state.selected ? s.selectedLineContent : s.lineContent}`}>
					{simpleContentsString(contents)}
				</td>
			</tr>
		);
	}
}

BlobLine.propTypes = {
	lineNumber: (props, propName, componentName) => {
		let v = React.PropTypes.number(props, propName, componentName);
		if (v) return v;
		if (typeof props.lineNumber !== "undefined" && (typeof props.oldLineNumber !== "undefined" || typeof props.newLineNumber !== "undefined")) {
			return new Error("If lineNumber is set, then oldLineNumber/newLineNumber (which are for diff hunks) may not be used");
		}
	},

	// Optional: for linking line numbers to the file they came from (e.g., in
	// ref snippets).
	repo: React.PropTypes.string,
	rev: React.PropTypes.string,
	path: React.PropTypes.string,

	// For diff hunks.
	oldLineNumber: React.PropTypes.number,
	newLineNumber: React.PropTypes.number,

	activeDef: React.PropTypes.string, // the def that the page is about
	activeDefNoRev: React.PropTypes.string, // activeDef without an "@rev" (if any)

	// startByte is the byte position of the first byte of contents. It is
	// required if annotations are specified, so that the annotations can
	// be aligned to the contents.
	startByte: (props, propName, componentName) => {
		if (props.annotations) return React.PropTypes.number.isRequired(props, propName, componentName);
	},
	contents: React.PropTypes.string,
	annotations: React.PropTypes.array,
	selected: React.PropTypes.bool,
	highlightedDef: React.PropTypes.string,
	className: React.PropTypes.string,
};

export default BlobLine;
