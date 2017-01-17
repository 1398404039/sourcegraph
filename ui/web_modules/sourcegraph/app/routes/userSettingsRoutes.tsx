import { PlainRoute } from "react-router";
import { rel } from "sourcegraph/app/routePatterns";
import { SettingsMain } from "sourcegraph/user/settings/SettingsMain";

export const userSettingsRoutes: PlainRoute[] = [
	{
		path: rel.settings,
		getComponents: (location, callback) => {
			callback(null, { main: SettingsMain });
		},
	},
];
