import React from "react";

import CSSModules from "react-css-modules";
import styles from "./styles/Dashboard.css";
import EventLogger from "sourcegraph/util/EventLogger";

/* See: https://developer.chrome.com/webstore/inline_installation */

class ChromeExtensionCTA extends React.Component {
	constructor(props) {
		super(props);
		this._handleClick = this._handleClick.bind(this);
		this._successHandler = this._successHandler.bind(this);
		this._failHandler = this._failHandler.bind(this);
	}

	componentDidMount() {
		EventLogger.logEvent("ChromeExtensionCTAPresented");
	}

	_successHandler() {
		EventLogger.logEvent("ChromeExtensionInstalled");
		if (this.props.onSuccess) this.props.onSuccess();
	}

	_failHandler() {
		EventLogger.logEvent("ChromeExtensionInstallFailed");
		if (this.props.onFail) this.props.onFail();
	}

	_handleClick() {
		EventLogger.logEvent("ChromeExtensionCTAClicked");
		if (global.chrome) {
			global.chrome.webstore.install("https://chrome.google.com/webstore/detail/dgjhfomjieaadpoljlnidmbgkdffpack", this._successHandler, this._failHandler);
		}
	}

	render() {
		return (
			<a styleName="cta-link" color="primary" outline={true} onClick={this._handleClick}>
				Install Chrome extension for GitHub.com (3,250 users)
			</a>
		);
	}
}

ChromeExtensionCTA.propTypes = {
	onSuccess: React.PropTypes.func,
	onFail: React.PropTypes.func,
};

export default CSSModules(ChromeExtensionCTA, styles);
