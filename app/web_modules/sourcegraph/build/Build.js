import React from "react";
import {formatDuration} from "sourcegraph/util/TimeAgo";

export function updatedAt(b) {
	return b.EndedAt || b.StartedAt || b.CreatedAt || null;
}

// buildStatus returns a textual status description for the build.
export function buildStatus(b) {
	if (b.Killed) return "Killed";
	if (b.Warnings) return "Warnings";
	if (b.Failure) return "Failed";
	if (b.Success) return "Succeeded";
	if (b.StartedAt && !b.EndedAt) return "Started";
	return "Queued";
}

// buildClass returns the CSS class for the build.
export function buildClass(b) {
	switch (buildStatus(b)) {
	case "Killed":
		return "danger";
	case "Warnings":
		return "warning";
	case "Failed":
		return "danger";
	case "Succeeded":
		return "success";
	case "Started":
		return "info";
	}
	return "default";
}

export function taskClass(task) {
	if (task.Success && !task.Skipped) return "success";
	if (task.Failure && !task.Skippe) return "danger";
	if (task.Warnings) return "warning";
	if (!task.Success && !task.Failure && !task.Skipped) return "info";
	return "default";
}

export function elapsed(buildOrTask) {
	if (!buildOrTask.StartedAt) return null;
	return (
		<div>
			{formatDuration((buildOrTask.EndedAt ? new Date(buildOrTask.EndedAt).getTime() : Date.now()) - new Date(buildOrTask.StartedAt).getTime())}
		</div>
	);
}
