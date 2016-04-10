import React from "react";

import CSSModules from "react-css-modules";
import styles from "./styles/input.css";

function Input(props) {
	const cls = props.styles[props.block ? "block" : "input"]; // eslint-disable-line react/prop-types
	return <input {...props} ref={props.domRef} className={cls} />;
}

Input.propTypes = {
	// block, if true, displays the input as a block element.
	block: React.PropTypes.bool,

	// domRef is like ref, but it is called with the <input> DOM element,
	// not this pure wrapper component. <Input domRef={...}> is equivalent
	// to <input ref={...}>.
	domRef: React.PropTypes.func,
};

export default CSSModules(Input, styles);
