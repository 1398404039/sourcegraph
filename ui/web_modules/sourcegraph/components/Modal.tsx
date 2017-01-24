import * as React from "react";

import { Router, RouterLocation } from "sourcegraph/app/router";
import { EventListener } from "sourcegraph/Component";
import * as styles from "sourcegraph/components/styles/modal.css";
import * as AnalyticsConstants from "sourcegraph/util/constants/AnalyticsConstants";
import { renderedOnBody } from "sourcegraph/util/renderedOnBody";

interface Props {
	onDismiss?: () => void;
	location?: RouterLocation;
}

interface State {
	originalOverflow: string | null;
}

export class ModalComp extends React.Component<Props, State> {
	private htmlElement: HTMLElement;

	constructor(props: Props) {
		super(props);
		this.state = {
			originalOverflow: document.body.style.overflowY,
		};
		this._onClick = this._onClick.bind(this);
		this._handleKeydown = this._handleKeydown.bind(this);
		this.bindBackingInstance = this.bindBackingInstance.bind(this);
	}

	componentDidMount(): void {
		this.setState({ originalOverflow: document.body.style.overflowY });
		document.body.style.overflowY = "hidden";
	}

	componentWillUnmount(): void {
		document.body.style.overflow = this.state.originalOverflow;
	}

	_onClick(e: React.MouseEvent<HTMLElement>): void {
		if (e.target === this.htmlElement) {
			if (this.props.onDismiss) {
				this.props.onDismiss();
			}
		}
	}

	_handleKeydown(e: KeyboardEvent): void {
		if (e.keyCode === 27 /* ESC */) {
			if (this.props.onDismiss) {
				this.props.onDismiss();
			}
		}
	}

	bindBackingInstance(el: HTMLElement): void {
		this.htmlElement = el;
	}

	render(): JSX.Element | null {
		return <div className={styles.container}
			ref={this.bindBackingInstance}
			onClick={this._onClick}>
			{this.props.children}
			<EventListener target={global.document} event="keydown" callback={this._handleKeydown} />
		</div>;
	}
}

let RenderedModal = renderedOnBody(ModalComp);

// setLocationModalState shows or hides a modal by setting the location.state.modal
// property to modalName if shown is true and null otherwise.
export function setLocationModalState(router: Router, location: RouterLocation, modalName: string, visible: boolean): void {
	router.replace(Object.assign({},
		location,
		{
			state: Object.assign({},
				location.state,
				{ modal: visible ? modalName : null },
			),
			query: Object.assign({},
				location.query,
				{ modal: visible ? modalName : undefined },
			),
		})
	);
}

// dismissModal creates a function that dismisses the modal by setting
// the location state's modal property to null.
export function dismissModal(modalName: string, location: RouterLocation, router: Router): any {
	return () => {
		// Log all modal dismissal events in a consistent way. Note that any additions of new "modalName"s will require new events to be created
		const eventObject = AnalyticsConstants.getModalDismissedEventObject(modalName);
		if (eventObject) {
			eventObject.logEvent();
		} else {
			// TODO(dan) ensure proper params
		}

		setLocationModalState(router, location, modalName, false);
	};
}

interface LocationStateModalProps {
	location: RouterLocation;
	// modalName is the name of the modal (location.{state,query}.modal value) that this
	// LocationStateToggleLink component toggles.
	modalName: string;
	onDismiss?: (e: any) => void;
	children?: JSX.Element[];
	router: Router;
	style?: React.CSSProperties;
}

// TODO(nicot): We are getting rid of this function below with the up and coming nicot modal refactor, so the casting I did below is temporary.
// LocationStateModal wraps <Modal> and uses a key on the location state
// to determine whether it is displayed. Use LocationStateModal with
// LocationStateToggleLink.
export function LocationStateModal({ location, modalName, children, onDismiss, style, router }: LocationStateModalProps): JSX.Element {
	const currentModal = (location.state && location.state["modal"]) ? location.state["modal"] : location.query["modal"];
	if (currentModal !== modalName) {
		return <span />;
	}

	const onDismiss2 = (e) => {
		dismissModal(modalName, location, router)();
		if (onDismiss) {
			onDismiss(e);
		}
	};

	return <RenderedModal onDismiss={onDismiss2} style={style} location={location} router={router}>
		{children}
	</RenderedModal>;
}
