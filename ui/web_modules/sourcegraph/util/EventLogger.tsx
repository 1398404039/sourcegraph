import * as React from "react";
import { Route } from "react-router";

import { context } from "sourcegraph/app/context";
import { getRouteParams, getRoutePattern, getViewName } from "sourcegraph/app/routePatterns";
import { Router, RouterLocation } from "sourcegraph/app/router";
import * as Dispatcher from "sourcegraph/Dispatcher";
import * as OrgActions from "sourcegraph/org/OrgActions";
import * as RepoActions from "sourcegraph/repo/RepoActions";
import { HubSpot } from "sourcegraph/tracking/HubSpotWrapper";
import { Intercom } from "sourcegraph/tracking/IntercomWrapper";
import * as UserActions from "sourcegraph/user/UserActions";
import * as AnalyticsConstants from "sourcegraph/util/constants/AnalyticsConstants";
import { experimentManager } from "sourcegraph/util/ExperimentManager";
import { Features } from "sourcegraph/util/features";
import { defPathToLanguage, getLanguageExtensionForPath } from "sourcegraph/util/inventory";
import * as optimizely from "sourcegraph/util/Optimizely";

class EventLoggerClass {
	_telligent: any = null;

	_dispatcherToken: any;
	_gaClientID: string;

	private CLOUD_TRACKING_APP_ID: string = "SourcegraphWeb";
	private PLATFORM: string = "Web";

	constructor() {
		// Listen for all Stores dispatches.
		// You must separately log "frontend" actions of interest,
		// with the relevant event properties.
		this._dispatcherToken = Dispatcher.Stores.register(this.__onDispatch.bind(this));

	}

	// init initializes Telligent and Intercom.
	init(): void {
		if (global.window) {
			this._telligent = global.window.telligent;

			let env = "development";
			let appId = "UnknownApp";
			if (context.version !== "dev" && context.trackingAppID) {
				env = "production";
				appId = context.trackingAppID;
			}

			this._telligent("newTracker", "sg", "sourcegraph-logging.telligentdata.com", {
				appId: appId,
				platform: "Web",
				encodeBase64: false,
				env: env,
				configUseCookies: true,
				useCookies: true,
				metadata: {
					gaCookies: true,
					performanceTiming: true,
					augurIdentityLite: true,
					webPage: true,
				},
			});
		}

		global.window.ga(function (tracker: any): any {
			this._gaClientID = tracker.get("clientId");
		}.bind(this));

		this._updateUser();
	}

	// _updateUser is be called whenever the user changes (on the initial page load).
	//
	// If any events have been buffered, it will flush them immediately.
	// If you do not call _updateUser or it is run on the server,
	// any subequent calls to logEvent or setUserProperty will be buffered.
	_updateUser(): void {
		const user = context.user;
		const emails = context.emails && context.emails.EmailAddrs || null;

		const primaryEmail = (emails && emails.filter(e => e.Primary).map(e => e.Email)[0]) || null;
		const optimizelyAttributes = { telligent_duid: this._getTelligentDuid() };
		const hubSpotAttributes = {};

		if (context.user) {
			this._setTrackerLoginInfo(context.user.Login);
			Intercom.setIntercomProperty("user_id", context.user.UID.toString());
			Intercom.setIntercomProperty("internal_user_id", context.user.UID.toString());
			hubSpotAttributes["user_id"] = context.user.Login;
			optimizelyAttributes["user_id"] = context.user.Login;
		}

		if (context.intercomHash) {
			Intercom.setIntercomProperty("user_hash", context.intercomHash);
			this.setUserProperty("user_hash", context.intercomHash);
		}

		Intercom.boot(context.trackingAppID !== this.CLOUD_TRACKING_APP_ID, context.trackingAppID);

		if (user) {
			if (user.Name) {
				Intercom.setIntercomProperty("name", user.Name);
				this.setUserProperty("display_name", user.Name);
				hubSpotAttributes["fullname"] = user.Name;
			}

			if (user.RegisteredAt) {
				this.setUserProperty("registered_at_timestamp", user.RegisteredAt);
				this.setUserProperty("registered_at", new Date(user.RegisteredAt).toDateString());
				Intercom.setIntercomProperty("created_at", new Date(user.RegisteredAt).getTime() / 1000);
				hubSpotAttributes["registered_at"] = new Date(user.RegisteredAt).toDateString();
			}

			if (user.Company) {
				this.setUserProperty("company", user.Company);
				Intercom.setIntercomProperty("company", user.Company);
				hubSpotAttributes["company"] = user.Company;
			}

			if (user.Location) {
				this.setUserProperty("location", user.Location);
				hubSpotAttributes["location"] = user.Location;
			}

			this.setUserProperty("is_private_code_user", context.hasPrivateGitHubToken() ? "true" : "false");
			this.setUserProperty("is_github_organization_authed", context.hasOrganizationGitHubToken() ? "true" : "false");
			hubSpotAttributes["is_private_code_user"] = context.hasPrivateGitHubToken() ? "true" : "false";
		}

		if (primaryEmail) {
			this.setUserProperty("email", primaryEmail);
			this.setUserProperty("emails", emails);
			Intercom.setIntercomProperty("email", primaryEmail);
			optimizelyAttributes["email"] = primaryEmail;
			hubSpotAttributes["email"] = primaryEmail;
			hubSpotAttributes["emails"] = emails ? emails.map(email => { return email.Email; }).join(",") : "";
		}

		if (optimizely.optimizelyApiService) {
			optimizely.optimizelyApiService.setUserAttributes(optimizelyAttributes);
		}
		HubSpot.setHubSpotProperties(hubSpotAttributes);
	}

	logout(): void {
		AnalyticsConstants.Events.Logout_Clicked.logEvent();

		// Prevent the next user who logs in (e.g., on a public terminal) from
		// seeing the previous user's Intercom messages.
		Intercom.shutdown();

		if (optimizely.optimizelyApiService) {
			optimizely.optimizelyApiService.logout();
		}
	}

	// Responsible for setting the login information for all event trackers
	_setTrackerLoginInfo(loginInfo: string): void {
		if (global.window.ga) {
			global.window.ga("set", "userId", loginInfo);
		}

		if (this._telligent) {
			this._telligent("setUserId", loginInfo);
		}

		Intercom.setIntercomProperty("business_user_id", loginInfo);
	}

	/*
	* Function to extract the Telligent user ID from the first-party cookie set by the Telligent JavaScript Tracker
	*
	* @return string or bool The ID string if the cookie exists or false if the cookie has not been set yet
	*/
	_getTelligentDuid(): string | null {
		let cookieName = "_te_";
		let matcher = new RegExp(cookieName + "id\\.[a-f0-9]+=([^;]+);?");
		let match = document.cookie.match(matcher);
		if (match && match[1]) {
			return match[1].split(".")[0];
		} else {
			return null;
		}
	}

	updateTrackerWithIdentificationProps(): any {
		if (!this._telligent || !context.hasChromeExtensionInstalled()) {
			return null;
		}

		let idProps = { detail: { deviceId: this._getTelligentDuid(), userId: context.user && context.user.Login } };
		if (global.window.ga) {
			this._telligent("addStaticMetadataObject", { deviceInfo: { GAClientId: this._gaClientID } });
			setTimeout(() => document.dispatchEvent(new CustomEvent("sourcegraph:identify", Object.assign(idProps, { gaClientId: this._gaClientID }))), 20);
		} else {
			setTimeout(() => document.dispatchEvent(new CustomEvent("sourcegraph:identify", idProps)), 20);
		}
	}

	// sets current user's properties
	setUserProperty(property: string, value: any): void {
		if (this._telligent) {
			this._telligent("addStaticMetadata", property, value, "userInfo");
		}
	}

	_decorateEventProperties(platformProperties: any): any {
		let optimizelyMetadata = {};
		if (optimizely.optimizelyApiService) {
			optimizelyMetadata = optimizely.optimizelyApiService.getOptimizelyMetadata();
		}
		const addtlPlatformProperties = {
			Platform: this.PLATFORM,
			is_authed: context.user ? "true" : "false",
			path_name: global.window && global.window.location && global.window.location.pathname ? global.window.location.pathname.slice(1) : ""
		};
		return Object.assign({}, platformProperties, addtlPlatformProperties, optimizelyMetadata);
	}

	// Use logViewEvent as the default way to log view events for Telligent and GA
	// location is the URL, page is the path.
	logViewEvent(title: string, page: string, eventProperties: any): void {
		if (context.userAgentIsBot || !page) {
			return;
		}

		this._logToConsole(title, Object.assign({}, this._decorateEventProperties(eventProperties), { page_name: page, page_title: title }));

		if (this._telligent) {
			this._telligent("track", "view", Object.assign({}, this._decorateEventProperties(eventProperties), { page_name: page, page_title: title }));
		}
	}

	// Default tracking call to all of our analytics servies.
	// By default, should only be called by AnalyticsConstants.LoggableEvent.logEvent()
	// Required fields: event
	// Optional fields: eventProperties
	logEventForCategory(event: any, eventProperties?: any): void {
		this._logEventForCategoryComponents(event.category, event.action, event.label, eventProperties);
	}

	_logEventForCategoryComponents(eventCategory: string, eventAction: string, eventLabel: string, eventProperties?: any): void {
		if (context.userAgentIsBot || !eventLabel) {
			return;
		}
		if (this._telligent) {
			this._telligent("track", eventAction, Object.assign({}, this._decorateEventProperties(eventProperties), { eventLabel: eventLabel, eventCategory: eventCategory, eventAction: eventAction }));
		}

		this._logToConsole(eventAction, Object.assign(this._decorateEventProperties(eventProperties), { eventLabel: eventLabel, eventCategory: eventCategory, eventAction: eventAction }));

		// Send event to ExperimentManager to determine if it should be tracked, and to send to Optimizely if so
		experimentManager.logEvent(eventLabel);

		// Log event on HubSpot (if a valid HubSpot event)
		HubSpot.logHubSpotEvent(eventLabel);

		if (global && global.window && global.window.ga) {
			global.window.ga("send", {
				hitType: "event",
				eventCategory: eventCategory || "",
				eventAction: eventAction || "",
				eventLabel: eventLabel,
			});
		}
	}

	_logToConsole(eventAction: string, object?: any): void {
		if (Features.eventLogDebug.isEnabled()) {
			console.debug("%cEVENT %s", "color: #aaa", eventAction, object); // tslint:disable-line
		}
	}

	// Tracking call for event level calls that we wish to track, but do not wish to impact bounce rate on our site for Google analytics.
	// An example of this would be the event that gets fired following a view event on a Repo that 404s. We fire a view event and then a 404 event.
	// By adding a non-interactive flag to the 404 event the page will correctly calculate bounce rate even with the additional event fired.
	logNonInteractionEventForCategory(eventObject: AnalyticsConstants.NonInteractionLoggableEvent, eventProperties?: any): void {
		if (context.userAgentIsBot || !eventObject.label) {
			return;
		}

		this._logToConsole(eventObject.action, Object.assign(this._decorateEventProperties(eventProperties), { eventLabel: eventObject.label, eventCategory: eventObject.category, eventAction: eventObject.action, nonInteraction: true }));

		if (this._telligent) {
			this._telligent("track", eventObject.action, Object.assign({}, this._decorateEventProperties(eventProperties), { eventLabel: eventObject.label, eventCategory: eventObject.category, eventAction: eventObject.action }));
		}

		if (global && global.window && global.window.ga) {
			global.window.ga("send", {
				hitType: "event",
				eventCategory: eventObject.category || "",
				eventAction: eventObject.action || "",
				eventLabel: eventObject.label,
				nonInteraction: true,
			});
		}
	}

	_dedupedArray(inputArray: Array<string>): Array<string> {
		return inputArray.filter(function (elem: string, index: number, self: any): any {
			return elem && (index === self.indexOf(elem));
		});
	}

	__onDispatch(action: any): void {
		switch (action.constructor) {
			case RepoActions.ReposFetched:
				if (action.isUserRepos) {
					if (action.data.Repos) {
						let languages: Array<string> = [];
						let repos: Array<string> = [];
						let repoOwners: Array<string> = [];
						let repoNames: Array<string> = [];
						for (let repo of action.data.Repos) {
							languages.push(repo["Language"]);
							repoNames.push(repo["Name"]);
							repoOwners.push(repo["Owner"]);
							repos.push(` ${repo["Owner"]}/${repo["Name"]}`);
						}

						this.setUserProperty("authed_languages_github", this._dedupedArray(languages));
						this.setUserProperty("num_repos_github", action.data.Repos.length);
						AnalyticsConstants.Events.RepositoryAuthedLanguagesGitHub_Fetched.logEvent({ "fetched_languages_github": this._dedupedArray(languages) });
						AnalyticsConstants.Events.RepositoryAuthedReposGitHub_Fetched.logEvent({ "fetched_repo_names_github": this._dedupedArray(repoNames), "fetched_repo_owners_github": this._dedupedArray(repoOwners), "fetched_repos_github": this._dedupedArray(repos) });
					}
				}
				break;
			case UserActions.BetaSubscriptionCompleted:
				if (action.eventObject) {
					action.eventObject.logEvent();
				}
				break;
			case OrgActions.OrgsFetched:
				let orgNames: Array<string> = [];
				if (action.data) {
					for (let orgs of action.data) {
						orgNames.push(orgs.Login);
						if (orgs.Login === "sourcegraph") {
							this.setUserProperty("is_employee", true);
							if (optimizely.optimizelyApiService) {
								optimizely.optimizelyApiService.setUserAttributes({ "is_employee": true });
							}
						}
					}
					HubSpot.setHubSpotProperties({ "authed_orgs_github": orgNames.join(",") });
					Intercom.setIntercomProperty("authed_orgs_github", orgNames);
					this.setUserProperty("authed_orgs_github", orgNames);
					AnalyticsConstants.Events.AuthedOrgsGitHub_Fetched.logEvent({ "fetched_orgs_github": orgNames });
				}
				break;
			case OrgActions.OrgMembersFetched:
				if (action.data && action.orgName) {
					let orgName: string = action.orgName;
					let orgMemberNames: string[] = [];
					let orgMemberEmails: string[] = [];
					for (let member of action.data) {
						orgMemberNames.push(member.Login);
						orgMemberEmails.push(member.Email || "");
					}
					AnalyticsConstants.Events.AuthedOrgMembersGitHub_Fetched.logEvent({ "fetched_org_github": orgName, "fetched_org_member_names_github": orgMemberNames, "fetched_org_member_emails_github": orgMemberEmails });
				}
				break;
			default:
				// All dispatched actions to stores will automatically be tracked by the eventName
				// of the action (if set). Override this behavior by including another case above.
				if (action.eventObject) {
					action.eventObject.logEvent();
				} else if (action.eventName) {
					this._logEventForCategoryComponents(AnalyticsConstants.EventCategories.Unknown, AnalyticsConstants.EventActions.Fetch, action.eventName);
				}
				break;
		}

		this._updateUser();
	}
}

export const EventLogger = new EventLoggerClass();

// withViewEventsLogged calls (this.context as any).eventLogger.logEvent when the
// location's pathname changes.
interface WithViewEventsLoggedProps {
	routes: Route[];
	location: RouterLocation;
}

export function withViewEventsLogged<P extends WithViewEventsLoggedProps>(component: React.ComponentClass<{}>): React.ComponentClass<{}> {
	class WithViewEventsLogged extends React.Component<P, {}> {
		static contextTypes: React.ValidationMap<any> = {
			router: React.PropTypes.object.isRequired,
		};

		context: {
			router: Router,
		};

		componentDidMount(): void {
			this._logView(this.props.routes, this.props.location);
			this._checkEventQuery();
		}

		componentWillReceiveProps(nextProps: P): void {
			// Greedily log page views. Technically changing the pathname
			// may match the same "view" (e.g. interacting with the directory
			// tree navigations will change your URL,  but not feel like separate
			// page events). We will log any change in pathname as a separate event.
			// NOTE: this will not log separate page views when query string / hash
			// values are updated.
			if (this.props.location.pathname !== nextProps.location.pathname) {
				this._logView(nextProps.routes, nextProps.location);
				// Greedily update the event logging tracker identity
				EventLogger.updateTrackerWithIdentificationProps();
			}

			this._checkEventQuery();
		}

		camelCaseToUnderscore(input: string): string {
			if (input.charAt(0) === "_") {
				input = input.substring(1);
			}

			return input.replace(/([A-Z])/g, ($1) => `_${$1.toLowerCase()}`);
		}

		_checkEventQuery(): void {
			// Allow tracking events that occurred externally and resulted in a redirect
			// back to Sourcegraph. Pull the event name out of the URL.
			const eventName = this.props.location.query["_event"];
			if (this.props.location.query && eventName) {
				// For login signup related metrics a channel will be associated with the signup.
				// This ensures we can track one metrics "SignupCompleted" and then query on the channel
				// for more granular metrics.
				let eventProperties = {};
				for (let key in this.props.location.query) {
					if (key !== "_event") {
						eventProperties[this.camelCaseToUnderscore(key)] = this.props.location.query[key];
					}
				}

				if (this.props.location.query["_githubAuthed"]) {
					EventLogger.setUserProperty("github_authed", this.props.location.query["_githubAuthed"]);
					if (eventName === "SignupCompleted") {
						AnalyticsConstants.Events.Signup_Completed.logEvent(eventProperties);
						if (context.user) {
							// When the user signs up. Fire off a request to get orgs and repos if they have the scope.
							Dispatcher.Backends.dispatch(new RepoActions.WantRepos("RemoteOnly=true", true));
							if (context.hasOrganizationGitHubToken()) {
								Dispatcher.Backends.dispatch(new OrgActions.WantOrgs(context.user.Login));
							}
						}
					} else if (eventName === "CompletedGitHubOAuth2Flow") {
						AnalyticsConstants.Events.OAuth2FlowGitHub_Completed.logEvent(eventProperties);
					}
				} else if (this.props.location.query["_invited_by_user"]) {
					EventLogger.setUserProperty("invited_by_user", this.props.location.query["_invited_by_user"]);
					AnalyticsConstants.Events.OrgEmailInvite_Clicked.logEvent(eventProperties);
				} else {
					EventLogger._logEventForCategoryComponents(AnalyticsConstants.EventCategories.External, AnalyticsConstants.EventActions.Redirect, eventName, eventProperties);
				}

				if (this.props.location.query["_org_invite"]) {
					EventLogger.setUserProperty("org_invite", this.props.location.query["_org_invite"]);
				}

				// Won't take effect until we call replace below, but prevents this
				// from being called 2x before the setTimeout block runs.
				delete this.props.location.query["_event"];
				delete this.props.location.query["_githubAuthed"];
				delete this.props.location.query["_org_invite"];
				delete this.props.location.query["_invited_by_user"];
				delete this.props.location.query["_def_info_def"];
				delete this.props.location.query["_repo"];
				delete this.props.location.query["_rev"];
				delete this.props.location.query["_path"];
				delete this.props.location.query["_source"];
				delete this.props.location.query["_githubCompany"];
				delete this.props.location.query["_githubName"];
				delete this.props.location.query["_githubLocation"];

				// Remove _event from the URL to canonicalize the URL and make it
				// less ugly.
				const locWithoutEvent = Object.assign({}, this.props.location, {
					query: Object.assign({}, this.props.location.query, { _event: undefined, _signupChannel: undefined, _onboarding: undefined, _githubAuthed: undefined, invited_by_user: undefined, org_invite: undefined, _def_info_def: undefined, _repo: undefined, _rev: undefined, _path: undefined, _source: undefined, _githubCompany: undefined, _githubName: undefined, _githubLocation: undefined }),
					state: Object.assign({}, this.props.location.state, { _onboarding: this.props.location.query["_onboarding"] }),
				});

				delete this.props.location.query["_signupChannel"];
				delete this.props.location.query["_onboarding"];

				(this.context as any).router.replace(locWithoutEvent);
			}
		}

		_logView(routes: Route[], location: RouterLocation): void {
			let eventProps: {
				url: string;
				referred_by_integration?: string;
				referred_by_browser_ext?: string;
				referred_by_sourcegraph_editor?: string;
				language?: string;
			};

			if (location.query && location.query["utm_source"] === "integration" && location.query["type"]) {
				eventProps = {
					// Alfred, ChromeExtension, FireFoxExtension, SublimeEditor, VIMEditor.
					referred_by_integration: location.query["type"],
					url: location.pathname,
				};
			} else if (location.query && location.query["utm_source"] === "chromeext") {
				// TODO:matt remove this once all plugins are switched to new version
				// This is temporarily here for backwards compat
				eventProps = {
					referred_by_browser_ext: "chrome",
					url: location.pathname,
				};
			} else if (location.query && location.query["utm_source"] === "browser-ext" && location.query["browser_type"]) {
				eventProps = {
					referred_by_browser_ext: location.query["browser_type"],
					url: location.pathname,
				};
			} else if (location.query && location.query["utm_source"] === "sourcegraph-editor" && location.query["editor_type"]) {
				eventProps = {
					url: location.pathname,
					referred_by_sourcegraph_editor: location.query["editor_type"],
				};
			} else {
				eventProps = {
					url: location.pathname,
				};
			}

			const routePattern = getRoutePattern(routes);
			const viewName = getViewName(routes);
			const routeParams = getRouteParams(routePattern, location.pathname);

			if (viewName) {
				if (viewName === "ViewBlob" && routeParams) {
					const filePath = routeParams.splat[routeParams.splat.length - 1];
					const lang = getLanguageExtensionForPath(filePath);
					if (lang) { eventProps.language = lang; }
				} else if ((viewName === "ViewDef" || viewName === "ViewDefInfo") && routeParams) {
					const defPath = routeParams.splat[routeParams.splat.length - 1];
					const lang = defPathToLanguage(defPath);
					if (lang) { eventProps.language = lang; }
				}

				EventLogger.logViewEvent(viewName, location.pathname, Object.assign({}, eventProps, { pattern: getRoutePattern(routes) }));
			} else {
				EventLogger.logViewEvent("UnmatchedRoute", location.pathname, Object.assign({}, eventProps, { pattern: getRoutePattern(routes) }));
			}
		}

		render(): JSX.Element | null {
			// This method fires a custom event to tell Optimize 360 to check if the current user should
			// receive any live A/B tests, and if so, to activate them. The Optimize 360 event handler is
			// idempotent, and only pings Google's remote server once per page load. By firing here, we
			// provide universal handling of live A/B tests, with no other frontend JavaScript code required
			// TODO (Dan): turned off to prevent Optimize from firing after an event has ended
			// activateDefaultExperiments();

			return React.createElement(component, this.props);
		}
	}
	return WithViewEventsLogged;
}
