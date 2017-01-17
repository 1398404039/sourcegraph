import { formatPattern } from "react-router/lib/PatternUtils";

import { RouteName, abs } from "sourcegraph/app/routePatterns";
import { RouteParams } from "sourcegraph/app/router";
import { RouterLocation } from "sourcegraph/app/router";

// urlTo produces the full URL, given a route and route parameters. The
// route names are defined in sourcegraph/app/routePatterns.
export function urlTo(name: RouteName, params: RouteParams): string {
	return formatPattern(`/${abs[name]}`, params);
}

export type oauthProvider = "github" | "google";

// urlToOAuth returns an OAuth initiate URL for given provider, scopes, returnTo.
export function urlToOAuth(provider: oauthProvider, scopes: string | null, returnTo: string | RouterLocation | null, newUserReturnTo: string | RouterLocation | null): string {
	scopes = scopes ? `scopes=${encodeURIComponent(scopes)}` : null;
	if (returnTo && typeof returnTo !== "string") {
		returnTo = `${returnTo.pathname}${returnTo.search}${returnTo.hash}`;
	}
	returnTo = returnTo && returnTo.toString();
	returnTo = returnTo ? `return-to=${encodeURIComponent(returnTo)}` : null;

	if (newUserReturnTo && typeof newUserReturnTo !== "string") {
		newUserReturnTo = `${newUserReturnTo.pathname}${newUserReturnTo.search}${newUserReturnTo.hash}`;
	}
	newUserReturnTo = newUserReturnTo && newUserReturnTo.toString();
	newUserReturnTo = newUserReturnTo ? `new-user-return-to=${encodeURIComponent(newUserReturnTo)}` : null;

	let q;
	if (scopes && returnTo && newUserReturnTo) {
		q = `${scopes}&${returnTo}&${newUserReturnTo}`;
	} else if (scopes && returnTo) {
		q = `${scopes}&${returnTo}`;
	} else if (scopes) {
		q = scopes;
	} else if (returnTo) {
		q = returnTo;
	}
	return `/-/${provider}-oauth/initiate${q ? `?${q}` : ""}`;
}

export const privateGitHubOAuthScopes = "read:org,repo,user:email";

export const privateGoogleOAuthScopes = "https://www.googleapis.com/auth/cloud-platform,https://www.googleapis.com/auth/userinfo.email,https://www.googleapis.com/auth/userinfo.profile";
