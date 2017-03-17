import * as React from "react";
import { Heading, Panel, TabItem, TabPanel, TabPanels, Table, Tabs } from "sourcegraph/components";
import * as base from "sourcegraph/components/styles/_base.css";

interface State {
	activeExample: number;
}

export class TableComponent extends React.Component<{}, State> {
	state: State = {
		activeExample: 0,
	};

	render(): JSX.Element | null {
		return (
			<div>
				<Tabs>
					<TabItem
						active={this.state.activeExample === 0}>
						<a href="#" onClick={(e) => {
							this.setState({ activeExample: 0 });
							e.preventDefault();
						}}>
							Default
						</a>
					</TabItem>
					<TabItem
						active={this.state.activeExample === 1}>
						<a href="#" onClick={(e) => {
							this.setState({ activeExample: 1 });
							e.preventDefault();
						}}>
							Bordered
						</a>
					</TabItem>
				</Tabs>

				<Panel hoverLevel="low">
					<TabPanels active={this.state.activeExample}>
						<TabPanel>
							<div className={base.pa4}>
								<Heading level={7} className={base.mb3} color="cool_mid_gray">Default table style</Heading>
								<Table style={{ width: "100%" }}>
									<thead>
										<tr>
											<td>Name</td>
											<td>User ID</td>
											<td>Email</td>
										</tr>
									</thead>
									<tbody>
										<tr>
											<td>Chelsea Otakan</td>
											<td>0</td>
											<td>chelsea@example.com</td>
										</tr>
										<tr>
											<td>Quinn Slack</td>
											<td>1</td>
											<td>sqs@example.com</td>
										</tr>
										<tr>
											<td>John Rothfels</td>
											<td>2</td>
											<td>john@example.com</td>
										</tr>
										<tr>
											<td>Renfred Harper</td>
											<td>3</td>
											<td>renfred@example.com</td>
										</tr>
									</tbody>
								</Table>
							</div>
							<hr />
							<code>
								<pre className={base.ph4} style={{ whiteSpace: "pre-wrap" }}>
									{
										`
<Table style={{width: "100%"}}>
	<thead>
		<tr>
			<td>Name</td>
			<td>User ID</td>
			<td>Email</td>
		</tr>
	</thead>
	<tbody>
		<tr>
			<td>Chelsea Otakan</td>
			<td>0</td>
			<td>chelsea@example.com</td>
		</tr>
		<tr>
			<td>Quinn Slack</td>
			<td>1</td>
			<td>sqs@example.com</td>
		</tr>
		<tr>
			<td>John Rothfels</td>
			<td>2</td>
			<td>john@example.com</td>
		</tr>
		<tr>
			<td>Renfred Harper</td>
			<td>3</td>
			<td>renfred@example.com</td>
		</tr>
	</tbody>
</Table>
	`
									}
								</pre>
							</code>
						</TabPanel>
						<TabPanel>
							<div className={base.pa4}>
								<Heading level={7} className={base.mb3} color="cool_mid_gray">Default table style</Heading>
								<Table style={{ width: "100%" }} bordered={true}>
									<thead>
										<tr>
											<td>Name</td>
											<td>User ID</td>
											<td>Email</td>
										</tr>
									</thead>
									<tbody>
										<tr>
											<td>Chelsea Otakan</td>
											<td>0</td>
											<td>chelsea@example.com</td>
										</tr>
										<tr>
											<td>Quinn Slack</td>
											<td>1</td>
											<td>sqs@example.com</td>
										</tr>
										<tr>
											<td>John Rothfels</td>
											<td>2</td>
											<td>john@example.com</td>
										</tr>
										<tr>
											<td>Renfred Harper</td>
											<td>3</td>
											<td>renfred@example.com</td>
										</tr>
									</tbody>
								</Table>
							</div>
							<hr />
							<code>
								<pre className={base.ph4} style={{ whiteSpace: "pre-wrap" }}>
									{
										`
<Table style={{width: "100%"}} bordered={true}>
	<thead>
		<tr>
			<td>Name</td>
			<td>User ID</td>
			<td>Email</td>
		</tr>
	</thead>
	<tbody>
		<tr>
			<td>Chelsea Otakan</td>
			<td>0</td>
			<td>chelsea@example.com</td>
		</tr>
		<tr>
			<td>Quinn Slack</td>
			<td>1</td>
			<td>sqs@example.com</td>
		</tr>
		<tr>
			<td>John Rothfels</td>
			<td>2</td>
			<td>john@example.com</td>
		</tr>
		<tr>
			<td>Renfred Harper</td>
			<td>3</td>
			<td>renfred@example.com</td>
		</tr>
	</tbody>
</Table>
	`
									}
								</pre>
							</code>
						</TabPanel>
					</TabPanels>
				</Panel>
			</div>
		);
	}
}
