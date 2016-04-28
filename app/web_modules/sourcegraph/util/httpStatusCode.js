// @flow

// httpStatusCode returns the HTTP status code that is most appropriate
// for the given Error (or 200 for null errors);
export default function httpStatusCode(err: ?Error): number {
	if (!err) return 200;
	if (err.response) return err.response.status;
	return 500;
}
