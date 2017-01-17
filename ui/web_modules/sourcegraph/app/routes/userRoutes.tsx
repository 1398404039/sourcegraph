import { PlainRoute } from "react-router";

import { rel } from "sourcegraph/app/routePatterns";
import { Login } from "sourcegraph/user/Login";
import { Signup } from "sourcegraph/user/Signup";

export const userRoutes: PlainRoute[] = [
	{
		path: rel.login,
		getComponents: (location, callback) => {
			callback(null, { main: Login });
		},
	},
	{
		path: rel.signup,
		getComponents: (location, callback) => {
			callback(null, { main: Signup });
		},
	}
];
