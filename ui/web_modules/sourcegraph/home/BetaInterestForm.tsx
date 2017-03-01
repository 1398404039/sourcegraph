import * as classNames from "classnames";
import * as React from "react";
import { context } from "sourcegraph/app/context";
import { RouterLocation } from "sourcegraph/app/router";
import { Component } from "sourcegraph/Component";
import { Button, CheckboxList, Input } from "sourcegraph/components";
import * as base from "sourcegraph/components/styles/_base.css";
import * as Dispatcher from "sourcegraph/Dispatcher";
import { editors, languageIDs, languageNames } from "sourcegraph/home/HomeUtils";
import * as styles from "sourcegraph/home/styles/BetaInterestForm.css";
import { langName } from "sourcegraph/Language";
import { SignupForm } from "sourcegraph/user/Signup";
import * as UserActions from "sourcegraph/user/UserActions";

interface Props {
	onSubmit?: () => void;
	className?: string;
	language?: string;
	location: RouterLocation;
	loginReturnTo: string;
	style?: React.CSSProperties;
}

type State = any;

export class BetaInterestForm extends Component<Props, State> {
	_dispatcherToken: string;

	// TODO(slimsag): these should be 'element' type?
	_fullName: any;
	_email: any;
	_company: any;
	_editors: any;
	_languages: any;
	_message: any;

	constructor(props: Props) {
		super(props);
		this._onChange = this._onChange.bind(this);
	}

	componentDidMount(): void {
		this._dispatcherToken = Dispatcher.Stores.register(this._onDispatch.bind(this));

		// Trigger _onChange now to save this.props.language if set.
		if (context.user && this.props.language) {
			this._onChange();
		}
	}

	componentWillUnmount(): void {
		Dispatcher.Stores.unregister(this._dispatcherToken);
	}

	reconcileState(state: State, props: Props): void {
		Object.assign(state, props);
	}

	_onDispatch(action: any): void {
		if (action instanceof UserActions.BetaSubscriptionCompleted) {
			this.setState({ resp: action.resp });
		}
	}

	_onChange(): void {
		window.localStorage["beta-interest-form"] = JSON.stringify({
			fullName: this._fullName["value"],
			email: this._email ? this._email["value"] : "",
			company: this._company["value"],
			editors: this._editors.selected(),
			languages: this._languages.selected(),
			message: this._message["value"],
		});
	}

	_sendForm(ev: any): void {
		ev.preventDefault();
		const name = this._fullName["value"];
		let firstName;
		let lastName;
		if (name) {
			const names = name.split(/\s+/);
			firstName = names[0];
			lastName = names.slice(1).join(" ");
		}

		if (this._editors.selected().length === 0) {
			this.setState({ formError: "Please select at least one preferred editor." });
			return;
		}
		if (this._languages.selected().length === 0) {
			this.setState({ formError: "Please select at least one preferred language." });
			return;
		}

		Dispatcher.Backends.dispatch(new UserActions.SubmitBetaSubscription(
			this._email ? this._email["value"].trim() : "",
			firstName || "",
			lastName || "",
			this._company["value"],
			this._languages.selected(),
			this._editors.selected(),
			this._message["value"].trim(),
		));
	}

	render(): JSX.Element | null {
		if (this.state.resp && !this.state.resp.Error) {
			// Display a "Close" button if there is an onSubmit handler.
			return (<span>
				<p>Success! Return to this page any time to update your favorite editors / languages!</p>
				<p>We'll contact you at <strong>{this.state.resp.EmailAddress}</strong> once a beta has begun.</p>
				{this.props.onSubmit && <Button block={true} type="submit" color="purple" onClick={this.props.onSubmit}>Close</Button>}
			</span>);
		}

		if (!context.user) {
			const newUserReturnTo = { pathname: this.props.loginReturnTo, hash: "" };

			return (<div className={styles.cta}>
				<p className={styles.p}>You must sign up to continue.</p>
				<SignupForm newUserReturnTo={newUserReturnTo} returnTo={this.props.loginReturnTo} location={this.props.location}></SignupForm>
			</div>);
		}

		let [className, language] = [this.props.className, this.props.language];
		let betaRegistered = false; // TODO
		let emails = context.emails && context.emails.EmailAddrs;

		let defaultFullName;
		let defaultEmail;
		let defaultCompany;
		let defaultMessage;
		let defaultEditors = [];
		let defaultLanguages: string[] = [];
		let ls = window.localStorage["beta-interest-form"];
		if (ls) {
			ls = JSON.parse(ls);
			defaultFullName = ls.fullName;
			defaultEmail = ls.email;
			defaultCompany = ls.company;
			defaultEditors = ls.editors;
			defaultLanguages = ls.languages;
			defaultMessage = ls.message;
		}

		if (language) {
			defaultLanguages.push(langName(language));
		}

		return (
			<div style={this.props.style}>
				{betaRegistered && <span>
					<p>You've already registered. We'll contact you once a beta matching your interests has begun.</p>
					<p>Feel free to update your favorite editors / languages using the form below.</p>
				</span>}
				<form className={className} onSubmit={this._sendForm.bind(this)} onChange={this._onChange}>
					<div className={styles.row}>
						<Input domRef={(c) => this._fullName = c} block={true} type="text" name="fullName" placeholder="Name" required={true} defaultValue={defaultFullName} />
					</div>
					{(!emails || emails.length === 0) && <div className={styles.row}>
						<Input domRef={(c) => this._email = c} block={true} type="email" name="email" placeholder="Email address" required={true} defaultValue={defaultEmail} />
					</div>}
					<div className={styles.row}>
						<Input domRef={(c) => this._company = c} block={true} type="text" name="company" placeholder="Company / organization" required={true} defaultValue={defaultCompany} />
					</div>
					<div className={styles.row}>
						<CheckboxList ref={(c) => this._editors = c} title="Preferred editors" name="editors" labels={editors} defaultValues={defaultEditors} />
					</div>
					<div className={styles.row}>
						<CheckboxList ref={(c) => this._languages = c} title="Preferred languages" name="languages" labels={languageNames} values={languageIDs} defaultValues={defaultLanguages} />
					</div>
					<div className={styles.row}>
						<textarea ref={(c) => this._message = c} className={styles.textarea} name="message" placeholder="Other / comments" defaultValue={defaultMessage}></textarea>
					</div>
					<div className={classNames(styles.row, base.pb4)}>
						<Button block={true} type="submit" color="purple">{betaRegistered ? "Update my interests" : "Participate in the beta"}</Button>
					</div>
					<div className={classNames(styles.row, base.pb4)}>
						{this.state.formError && <strong>{this.state.formError}</strong>}
						{this.state.resp && this.state.resp.Error && <div>{this.state.resp.Error.body}</div>}
					</div>
				</form>
			</div>
		);
	}
}
