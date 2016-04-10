// @flow

// singleflightFetch is a wrapper for fetch that suppresses
// duplicate calls. At most one call to fetch with the given arguments may
// be in-flight at any given time. If there are subsequent calls while the
// first call is still in flight, those callers receive the same promise
// returned by the first call. The Response is cloned so that they may
// read the body.
export function singleflightFetch(fetch: typeof fetch): typeof fetch {
	const inFlight: {[key: string]: boolean} = {};
	type WaitingFetch = {
		resolve: (result: Promise<Response> | Response) => void,
		reject: (err: any) => void
	};
	const waiting: {[key: string]: Array<WaitingFetch>} = {};
	const done = (key: string) => {
		delete inFlight[key];
		const waitingFetches = waiting[key];
		delete waiting[key];
		return waitingFetches;
	};
	return (input, options) => {
		// Don't handle complex requests.
		if (typeof input !== "string" || options) return fetch(input, options);

		const key: string = input;

		if (inFlight[key]) {
			if (!waiting[key]) waiting[key] = [];
			return new Promise((resolve, reject) => {
				waiting[key].push({resolve, reject});
			});
		}

		const f = fetch(input, options);
		inFlight[key] = true;
		return f.then((resp) => {
			done(key).forEach(({resolve}) => {
				resolve(resp.clone());
			});
			return resp;
		}).catch((err) => {
			done(key).forEach(({reject}) => {
				reject(err);
			});
			return err;
		});
	};
}
