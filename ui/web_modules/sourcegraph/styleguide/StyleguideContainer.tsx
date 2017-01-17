import * as React from "react";
import { Link } from "react-router";
import { Affix, FlexContainer, Heading, Hero, TabItem, Tabs } from "sourcegraph/components";
import { whitespace } from "sourcegraph/components/utils/index";
import { ComponentsContainer } from "sourcegraph/styleguide/ComponentsContainer";
import * as styles from "sourcegraph/styleguide/styles/StyleguideContainer.css";

export function StyleguideContainer(props: {}): JSX.Element {
	const navHeadingSx = {
		marginLeft: whitespace[3],
		paddingLeft: whitespace[3],
		marginTop: whitespace[4],
	};

	return (
		<div className={styles.bg_near_white}>
			<Hero color="purple" pattern="objects">
				<Heading level={2} color="white">The Graph Guide</Heading>
				<p style={{
					marginLeft: "auto",
					marginRight: "auto",
					maxWidth: 560,
					textAlign: "center",
				}}>
					Welcome to the Graph Guide – a living guide to Sourcegraph's brand identity, voice, visual style, and approach to user experience and user interfaces.
					</p>
			</Hero>
			<FlexContainer className={styles.container_fixed}>
				<Affix offset={20} style={{ flex: "0 0 240px", order: 9999 }}>
					<Tabs style={{ marginLeft: whitespace[5] }} direction="vertical">
						<TabItem direction="vertical" color="purple">
							<Link to={{ pathname: "styleguide", hash: "principles" }}>Principles</Link>
						</TabItem>

						<Heading level={7} style={navHeadingSx}>Brand</Heading>
						<TabItem direction="vertical" color="purple">
							<Link to={{ pathname: "styleguide", hash: "brand-voice" }}>Voice</Link>
						</TabItem>
						<TabItem direction="vertical" color="purple">
							<Link to={{ pathname: "styleguide", hash: "brand-logo" }}>Logo and Logotype</Link>
						</TabItem>

						<Heading level={7} style={navHeadingSx}>Layout Components</Heading>
						<TabItem direction="vertical" color="purple">
							<Link to={{ pathname: "styleguide", hash: "layout-flexcontainer" }}>FlexContainer</Link>
						</TabItem>
						<TabItem direction="vertical" color="purple">
							<Link to={{ pathname: "styleguide", hash: "layout-affix" }}>Affix</Link>
						</TabItem>

						<Heading level={7} style={navHeadingSx}>UI Components</Heading>
						<TabItem direction="vertical" color="purple">
							<Link to={{ pathname: "styleguide", hash: "components-buttons" }}>Buttons</Link>
						</TabItem>
						<TabItem direction="vertical" color="purple">
							<Link to={{ pathname: "styleguide", hash: "components-forms" }}>Forms</Link>
						</TabItem>
						<TabItem direction="vertical" color="purple">
							<Link to={{ pathname: "styleguide", hash: "components-headings" }}>Headings</Link>
						</TabItem>
						<TabItem direction="vertical" color="purple">
							<Link to={{ pathname: "styleguide", hash: "components-list" }}>Lists</Link>
						</TabItem>
						<TabItem direction="vertical" color="purple">
							<Link to={{ pathname: "styleguide", hash: "components-panels" }}>Panels</Link>
						</TabItem>
						<TabItem direction="vertical" color="purple">
							<Link to={{ pathname: "styleguide", hash: "components-stepper" }}>Stepper</Link>
						</TabItem>
						<TabItem direction="vertical" color="purple">
							<Link to={{ pathname: "styleguide", hash: "components-symbols" }}>Symbols</Link>
						</TabItem>
						<TabItem direction="vertical" color="purple">
							<Link to={{ pathname: "styleguide", hash: "components-table" }}>Table</Link>
						</TabItem>
						<TabItem direction="vertical" color="purple">
							<Link to={{ pathname: "styleguide", hash: "components-tabs" }}>Tabs</Link>
						</TabItem>
						<TabItem direction="vertical" color="purple">
							<Link to={{ pathname: "styleguide", hash: "components-user" }}>User</Link>
						</TabItem>
						<TabItem direction="vertical" color="purple">
							<Link to={{ pathname: "styleguide", hash: "components-repository-card" }}>Repository Card</Link>
						</TabItem>
					</Tabs>
				</Affix>
				<ComponentsContainer />
			</FlexContainer>
		</div>
	);
}
