import "sourcegraph/components/styles/_normalize.css";

import * as React from "react";
import Helmet from "react-helmet";
import {PlainRoute} from "react-router";

import {context} from "sourcegraph/app/context";
import {GlobalNav} from "sourcegraph/app/GlobalNav";
import * as styles from "sourcegraph/app/styles/App.css";
import {withViewEventsLogged} from "sourcegraph/util/EventLogger";

import {homeRoutes} from "sourcegraph/app/routes/homeRoutes";
import {pageRoutes} from "sourcegraph/app/routes/pageRoutes";
import {repoRoutes} from "sourcegraph/app/routes/repoRoutes";
import {styleguideRoutes} from "sourcegraph/app/routes/styleguideRoutes";
import {userRoutes} from "sourcegraph/app/routes/userRoutes";
import {userSettingsRoutes} from "sourcegraph/app/routes/userSettingsRoutes";

interface Props {
	main: JSX.Element;
	location: any;
	params?: any;
}

export class App extends React.Component<Props, {}> {
	render(): JSX.Element {
		let className = styles.main_container;
		if (!context.user && location.pathname === "/") {
			className = styles.main_container_homepage;
		}
		return (
			<div className={className}>
				<Helmet titleTemplate="%s · Sourcegraph" defaultTitle={document.title} />
				<GlobalNav params={this.props.params} location={this.props.location} />
				{this.props.main}
			</div>
		);
	}
}

export const rootRoute: PlainRoute = {
	path: "/",
	component: withViewEventsLogged(App),
	getIndexRoute: (location, callback) => {
		callback(null, homeRoutes);
	},
	getChildRoutes: (location, callback) => {
		callback(null, [
			...pageRoutes,
			...styleguideRoutes,
			...homeRoutes,
			...userRoutes,
			...userSettingsRoutes,
			...repoRoutes,
		]);
	},
};
