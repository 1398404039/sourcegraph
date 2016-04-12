import React from "react";
import Component from "sourcegraph/Component";
import s from "sourcegraph/def/styles/Def.css";
import {qualifiedNameAndType} from "sourcegraph/def/Formatter";

// These variables are needed to intialize the tooltips position to the current
// position of the mouse without a mousemove event.
let cursorX;
let cursorY;
if (typeof document !== "undefined") {
	// TODO(autotest) support document object.
	document.addEventListener("mousemove", (event) => {
		cursorX = event.clientX;
		cursorY = event.clientY;
	}, false);
}

class DefTooltip extends Component {
	constructor(props) {
		super(props);
		this._updatePosition = this._updatePosition.bind(this);
	}

	componentDidMount() {
		document.addEventListener("mousemove", this._updatePosition);
		this._updatePosition({clientY: cursorY, clientX: cursorX}); // Initialize position
	}

	componentWillUnmount() {
		this._elem = null;
		document.removeEventListener("mousemove", this._updatePosition);
	}

	reconcileState(state, props) {
		Object.assign(state, props);
	}

	_updatePosition(event) {
		if (!this._elem) return;
		if (typeof window !== "undefined") {
			window.requestAnimationFrame(() => {
				if (!this._elem) return;
				this._elem.style.top = `${event.clientY + 15}px`;
				this._elem.style.left = `${Math.min(event.clientX + 15, window.innerWidth - 380)}px`;
			});
		}
	}

	render() {
		let def = this.state.def;
		return (
			<div ref={(e) => this._elem = e} className={s.tooltip}>
				<div key="title" className={s.tooltipTitle}>{qualifiedNameAndType(def)}</div>,
				<div key="content" className={s.content}>
					{def && def.DocHTML && <div className={s.doc} dangerouslySetInnerHTML={def && def.DocHTML}></div>}
					{def && def.Repo !== this.state.currentRepo && <span className={s.repo}>{def.Repo}</span>}
				</div>
			</div>
		);
	}
}

DefTooltip.propTypes = {
	// currentRepo is the repo of the file that's currently being displayed, if any.
	currentRepo: React.PropTypes.string,

	def: React.PropTypes.object,
};

export default DefTooltip;
