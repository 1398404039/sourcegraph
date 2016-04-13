// @flow

import React from "react";
import {Link} from "react-router";

import Component from "sourcegraph/Component";

import urlTo from "sourcegraph/util/urlTo";
import breadcrumb from "sourcegraph/util/breadcrumb";

import CSSModules from "react-css-modules";
import styles from "sourcegraph/components/styles/breadcrumb.css";

class RepoNavContext extends Component {
	static propTypes = {
		params: React.PropTypes.object.isRequired,
	};

	reconcileState(state, props) {
		Object.assign(state, props);
	}

	render() {
		let blobPath = this.props.params.splat[1];
		if (!blobPath) return null;
		let pathParts = blobPath.split("/");
		let pathBreadcrumb = breadcrumb(
			`/${blobPath}`,
			(i) => <span key={i} styleName="sep">/</span>,
			(path, component, i, isLast) => (
				<Link to={isLast ?
					urlTo("blob", {...this.state.params}) :
					urlTo("tree", {
						...this.state.params,
						splat: [this.state.params.splat[0], pathParts.slice(0, i).join("/")],
					})}
					key={i}
					styleName={isLast ? "active" : "inactive"}>
					{component}
				</Link>
			),
		);

		return (
			<span>{pathBreadcrumb}</span>
		);
	}
}

export default CSSModules(RepoNavContext, styles);
