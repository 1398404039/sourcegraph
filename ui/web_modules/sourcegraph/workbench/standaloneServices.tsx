import { SimpleEditorService, StandaloneCommandService } from "vs/editor/browser/standalone/simpleServices";
import { IMenuService } from "vs/platform/actions/common/actions";
import { MenuService } from "vs/platform/actions/common/menuService";
import { ICommandService } from "vs/platform/commands/common/commands";
import { ContextKeyService } from "vs/platform/contextkey/browser/contextKeyService";
import { IContextKeyService } from "vs/platform/contextkey/common/contextkey";
import { ContextMenuService } from "vs/platform/contextview/browser/contextMenuService";
import { IContextMenuService, IContextViewService } from "vs/platform/contextview/browser/contextView";
import { ContextViewService } from "vs/platform/contextview/browser/contextViewService";
import { IEditorService } from "vs/platform/editor/common/editor";
import { IInstantiationService } from "vs/platform/instantiation/common/instantiation";
import { ServiceCollection } from "vs/platform/instantiation/common/serviceCollection";

export function standaloneServices(container: HTMLElement, services: ServiceCollection): void {
	const instantiationService = services.get(IInstantiationService) as IInstantiationService;

	const set = (identifier, impl) => {
		const instance = instantiationService.createInstance(impl);
		services.set(identifier, instance);
	};

	if (!services.has(IEditorService)) {
		services.set(IEditorService, new SimpleEditorService());
	}

	set(IContextKeyService, ContextKeyService);
	set(ICommandService, StandaloneCommandService);

	// The ContextViewService must be aware of the entire window (for absolute element positioning), not just
	// the workbench shell.
	const ctxView = instantiationService.createInstance(ContextViewService, document.querySelector("body") as HTMLElement);
	services.set(IContextViewService, ctxView);

	set(IContextMenuService, ContextMenuService);
	set(IMenuService, MenuService);
}
