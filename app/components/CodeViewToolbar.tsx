import { ActionsNavItems } from '@sourcegraph/extensions-client-common/lib/app/actions/ActionsNavItems'
import { ExtensionsProps } from '@sourcegraph/extensions-client-common/lib/context'
import { CXPControllerProps } from '@sourcegraph/extensions-client-common/lib/cxp/controller'
import { ISite, IUser } from '@sourcegraph/extensions-client-common/lib/schema/graphqlschema'
import {
    ConfigurationCascadeProps,
    ConfigurationSubject,
    Settings,
} from '@sourcegraph/extensions-client-common/lib/settings'
import { ContributableMenu } from 'cxp/module/protocol'
import * as React from 'react'
import { Subscription } from 'rxjs'
import { SimpleCXPFns } from '../backend/lsp'
import { fetchCurrentUser, fetchSite } from '../backend/server'
import { CodeIntelStatusIndicator } from './CodeIntelStatusIndicator'
import { OpenOnSourcegraph } from './OpenOnSourcegraph'

export interface ButtonProps {
    className: string
    style: React.CSSProperties
    iconStyle?: React.CSSProperties
}

interface CodeViewToolbarProps
    extends Partial<ExtensionsProps<ConfigurationSubject, Settings>>,
        Partial<CXPControllerProps<ConfigurationSubject, Settings>> {
    repoPath: string
    filePath: string

    baseCommitID: string
    baseRev?: string
    headCommitID?: string
    headRev?: string

    onEnabledChange?: (enabled: boolean) => void

    buttonProps: ButtonProps
    actionsNavItemClassProps?: {
        listClass?: string
        actionItemClass?: string
    }
    simpleCXPFns: SimpleCXPFns
}

interface CodeViewToolbarState extends ConfigurationCascadeProps<ConfigurationSubject, Settings> {
    site?: ISite
    currentUser?: IUser
}

export class CodeViewToolbar extends React.Component<CodeViewToolbarProps, CodeViewToolbarState> {
    public state: CodeViewToolbarState = {
        configurationCascade: { subjects: [], merged: {} },
    }

    private subscriptions = new Subscription()

    public componentDidMount(): void {
        if (this.props.extensions) {
            this.subscriptions.add(
                this.props.extensions.context.configurationCascade.subscribe(
                    configurationCascade => this.setState({ configurationCascade }),
                    err => console.error(err)
                )
            )
        }
        this.subscriptions.add(fetchSite().subscribe(site => this.setState(() => ({ site }))))
        this.subscriptions.add(fetchCurrentUser().subscribe(currentUser => this.setState(() => ({ currentUser }))))
    }

    public componentWillUnmount(): void {
        this.subscriptions.unsubscribe()
    }

    public render(): JSX.Element | null {
        const { site, currentUser } = this.state
        return (
            <div style={{ display: 'inline-flex', verticalAlign: 'middle', alignItems: 'center' }}>
                <ul className={`nav ${this.props.extensions ? 'pr-1' : ''}`}>
                    {this.props.cxpController &&
                        this.props.extensions && (
                            <div className="BtnGroup">
                                <ActionsNavItems
                                    menu={ContributableMenu.EditorTitle}
                                    cxpController={this.props.cxpController}
                                    extensions={this.props.extensions}
                                    listClass="BtnGroup"
                                    actionItemClass="btn btn-sm tooltipped tooltipped-n BtnGroup-item"
                                />
                            </div>
                        )}
                </ul>
                <CodeIntelStatusIndicator
                    key="code-intel-status"
                    userIsSiteAdmin={currentUser ? currentUser.siteAdmin : false}
                    repoPath={this.props.repoPath}
                    commitID={this.props.baseCommitID}
                    filePath={this.props.filePath}
                    onChange={this.props.onEnabledChange}
                    simpleCXPFns={this.props.simpleCXPFns}
                    site={site}
                />
                <OpenOnSourcegraph
                    label={`View File${this.props.headCommitID ? ' (base)' : ''}`}
                    ariaLabel="View file on Sourcegraph"
                    openProps={{
                        repoPath: this.props.repoPath,
                        filePath: this.props.filePath,
                        rev: this.props.baseRev || this.props.baseCommitID,
                        query: this.props.headCommitID
                            ? {
                                  diff: {
                                      rev: this.props.baseCommitID,
                                  },
                              }
                            : undefined,
                    }}
                    className={this.props.buttonProps.className}
                    style={this.props.buttonProps.style}
                    iconStyle={this.props.buttonProps.iconStyle}
                />
                {this.props.headCommitID && (
                    <OpenOnSourcegraph
                        label={'View File (head)'}
                        ariaLabel="View file on Sourcegraph"
                        openProps={{
                            repoPath: this.props.repoPath,
                            filePath: this.props.filePath,
                            rev: this.props.headRev || this.props.headCommitID,
                            query: {
                                diff: {
                                    rev: this.props.baseCommitID,
                                },
                            },
                        }}
                        className={this.props.buttonProps.className}
                        style={this.props.buttonProps.style}
                        iconStyle={this.props.buttonProps.iconStyle}
                    />
                )}
            </div>
        )
    }
}
