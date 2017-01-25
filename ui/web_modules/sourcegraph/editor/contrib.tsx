import { CancellationToken } from "vs/base/common/cancellation";
import { getLanguages, onLanguage, registerCodeLensProvider, registerDefinitionProvider, registerHoverProvider, registerReferenceProvider } from "vs/editor/browser/standalone/standaloneLanguages";
import { Position } from "vs/editor/common/core/position";
import { Range } from "vs/editor/common/core/range";
import { IPosition, IRange, IReadOnlyModel } from "vs/editor/common/editorCommon";
import * as modes from "vs/editor/common/modes";
import { Definition, Hover, Location, ReferenceContext } from "vs/editor/common/modes";

import { URIUtils } from "sourcegraph/core/uri";
import { AuthorshipCodeLens } from "sourcegraph/editor/authorshipCodeLens";
import * as lsp from "sourcegraph/editor/lsp";
import { modes as supportedModes } from "sourcegraph/editor/modes";
import * as AnalyticsConstants from "sourcegraph/util/constants/AnalyticsConstants";

supportedModes.forEach(mode => {
	onLanguage(mode, () => {
		registerHoverProvider(mode, new HoverProvider());
		registerDefinitionProvider(mode, new DefinitionProvder());
		registerReferenceProvider(mode, new ReferenceProvider());
	});
});

getLanguages().forEach(({ id }) => {
	onLanguage(id, () => {
		registerCodeLensProvider(id, new AuthorshipCodeLens());
	});
});

export function normalisePosition(model: IReadOnlyModel, position: IPosition): IPosition {
	const word = model.getWordAtPosition(position);
	if (!word) {
		return position;
	}
	// We always hover/j2d on the middle of a word. This is so multiple requests for the same word
	// result in a lookup on the same position.
	return {
		lineNumber: position.lineNumber,
		column: Math.floor((word.startColumn + word.endColumn) / 2),
	};
}

export class ReferenceProvider implements modes.ReferenceProvider {

	provideReferences(model: IReadOnlyModel, position: Position, context: ReferenceContext, token: CancellationToken): Location[] | Thenable<Location[]> {
		return lsp.send(model, "textDocument/references", {
			textDocument: { uri: model.uri.toString(true) },
			position: lsp.toPosition(position),
			context: { includeDeclaration: false },
		})
			.then((resp) => resp ? resp.result : null)
			.then((resp: lsp.Location | lsp.Location[] | null) => {
				if (!resp || Object.keys(resp).length === 0) {
					return null;
				}

				const { repo, rev, path } = URIUtils.repoParams(model.uri);
				AnalyticsConstants.Events.CodeReferences_Viewed.logEvent({ repo, rev: rev || "", path });

				const locs: lsp.Location[] = resp instanceof Array ? resp : [resp];
				return locs.map(lsp.toMonacoLocation);
			});
	}

}

export class HoverProvider implements modes.HoverProvider {

	provideHover(model: IReadOnlyModel, origPosition: Position): Thenable<Hover> {
		const position = normalisePosition(model, origPosition);
		const word = model.getWordAtPosition(position);

		const flight = lsp.send(model, "textDocument/hover", {
			textDocument: { uri: model.uri.toString(true) },
			position: lsp.toPosition(position),
		})
			.then(resp => {
				if (!resp || !resp.result || !resp.result.contents || resp.result.contents.length === 0) {
					return { contents: [] }; // if null, strings, whitespace, etc. will show a perpetu-"Loading..." tooltip
				}

				const { repo, rev, path } = URIUtils.repoParams(model.uri);
				AnalyticsConstants.Events.CodeToken_Hovered.logEvent({
					repo: repo,
					rev: rev || "",
					path: path,
					language: model.getModeId(),
				}
				);

				let range: IRange;
				if (resp.result.range) {
					range = lsp.toMonacoRange(resp.result.range);
				} else {
					range = new Range(position.lineNumber, word ? word.startColumn : position.column, position.lineNumber, word ? word.endColumn : position.column);
				}
				const contents = resp.result.contents instanceof Array ? resp.result.contents : [resp.result.contents];
				for (const c of contents) {
					if (c.value && c.value.length > 300) {
						c.value = c.value.slice(0, 300) + "...";
					}
				}

				// For some reason, this actually renders Markdown correctly
				// (code is monospace, prose is sans-serif), whereas without
				// this, those are rendered in the opposite ways (code is
				// sans-serif, prose is monospace).
				for (let i = 0; i < contents.length; i++) {
					if (contents[i].language === "markdown") {
						contents[i] = contents[i].value;
					}
				}

				contents.push("**Click to view references**");
				return {
					contents: contents,
					range,
				};
			});

		return flight;
	}

}

type result = Thenable<Definition | null>;

export class DefinitionProvder implements modes.DefinitionProvider {

	provideDefinition(model: IReadOnlyModel, origPosition: Position, token: CancellationToken): result {
		const position = normalisePosition(model, origPosition);
		const flight = lsp.send(model, "textDocument/definition", {
			textDocument: { uri: model.uri.toString(true) },
			position: lsp.toPosition(position),
		})
			.then((resp) => resp ? resp.result : null)
			.then((resp: lsp.Location | lsp.Location[] | null) => {
				if (!resp) {
					return null;
				}

				const locs: lsp.Location[] = resp instanceof Array ? resp : [resp];
				const translatedLocs: Location[] = locs
					.filter((loc) => Object.keys(loc).length !== 0)
					.map(lsp.toMonacoLocation);

				// TODO check that doesn't error when editor is disposed.
				return translatedLocs;
			});

		return flight;
	}

}
