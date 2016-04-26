// @flow

import {rel} from "sourcegraph/app/routePatterns";
import type {Route} from "react-router";

export type User = {
	UID: number;
	Login: string;
};

export type AuthInfo = {
	UID: number;
	Login: string;
};

const login = {
	getComponents: (location, callback) => {
		require.ensure([], (require) => {
			callback(null, {
				main: require("sourcegraph/user/Login").default,
			});
		});
	},
};
const signup = {
	getComponents: (location, callback) => {
		require.ensure([], (require) => {
			callback(null, {
				main: require("sourcegraph/user/Signup").default,
			});
		});
	},
};
const forgot = {
	getComponents: (location, callback) => {
		require.ensure([], (require) => {
			callback(null, {
				main: require("sourcegraph/user/ForgotPassword").default,
			});
		});
	},
};
const reset = {
	getComponents: (location, callback) => {
		require.ensure([], (require) => {
			callback(null, {
				main: require("sourcegraph/user/ResetPassword").default,
			});
		});
	},
};

export const routes: Array<Route> = [
	{
		...login,
		path: rel.login,
	},
	{
		...signup,
		path: rel.signup,
	},
	{
		...forgot,
		path: rel.forgot,
	},
	{
		...reset,
		path: rel.reset,
	},
];
