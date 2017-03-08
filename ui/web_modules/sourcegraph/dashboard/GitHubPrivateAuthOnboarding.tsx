// tslint:disable: typedef ordered-imports

import * as React from "react";
import * as base from "sourcegraph/components/styles/_base.css";
import * as classNames from "classnames";
import * as colors from "sourcegraph/components/styles/_colors.css";
import * as styles from "sourcegraph/dashboard/styles/Dashboard.css";
import * as typography from "sourcegraph/components/styles/_typography.css";
import { PageTitle } from "sourcegraph/components/PageTitle";
import { Heading, Panel } from "sourcegraph/components";
import { GitHubAuthButton } from "sourcegraph/components/GitHubAuthButton";
import { whitespace } from "sourcegraph/components/utils/index";
import { Events } from "sourcegraph/tracking/constants/AnalyticsConstants";
import { context } from "sourcegraph/app/context";

interface Props {
	location?: any;
	privateCodeAuthed?: any;
	completeStep?: any;
}

type State = any;

export class GitHubPrivateAuthOnboarding extends React.Component<Props, State> {
	static contextTypes: React.ValidationMap<any> = {
		router: React.PropTypes.object.isRequired,
	};

	_skipClicked() {
		Events.PrivateAuthGitHub_Skipped.logEvent({ page_name: "GitHubPrivateCodeOnboarding" });
		this.props.completeStep();
	}

	render(): JSX.Element | null {
		if (this.props.privateCodeAuthed) {
			this.props.completeStep();
			return null;
		}

		return (
			<div>
				<PageTitle title="Home" />
				<div className={styles.onboarding_container}>
					<Panel className={classNames(base.pb3, base.ph4, base.ba, base.br2, colors.b__cool_pale_gray)}>
						<Heading style={{ paddingTop: whitespace[5] }} align="center" level={3}>
							Browse your private code with Sourcegraph
						</Heading>
						<div className={styles.user_actions} style={{ maxWidth: "380px" }}>
							<p className={classNames(typography.tc, base.mt3, base.mb2, typography.f6, colors.cool_gray_8)} >
								Enable Sourcegraph on any private GitHub repositories for a better coding experience
							</p>
							<div className={classNames(base.pv5)}>
								<img width={332} style={{ marginBottom: "-95px" }} src={`${context.assetsRoot}/img/Dashboard/OnboardingRepos.png`}></img>
								<GitHubAuthButton pageName={"GitHubPrivateCodeOnboarding"} scope="private" returnTo={this.props.location} className={styles.github_button}>Add private repositories</GitHubAuthButton> to start a 14-day trial.
							</div>
							<p>
								<a onClick={this._skipClicked.bind(this)}>Skip</a>
							</p>
						</div>
					</Panel>
				</div>
			</div>
		);
	}
}
