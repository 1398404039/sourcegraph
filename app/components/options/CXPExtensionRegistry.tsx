import { ExtensionsList } from '@sourcegraph/extensions-client-common/lib/extensions/manager/ExtensionsList'
import {
    ConfigurationCascadeProps,
    ConfigurationSubject,
    Settings,
} from '@sourcegraph/extensions-client-common/lib/settings'
import * as React from 'react'
import { RouteComponentProps } from 'react-router-dom'
import { Subscription } from 'rxjs'
import { createExtensionsContextController } from '../../../app/backend/extensions'
import { BrowserSettingsEditor } from '../../../chrome/extension/cxp'
import { GQL } from '../../../types/gqlschema'

interface OptionsPageProps extends RouteComponentProps<{}> {}
interface OptionsPageState extends ConfigurationCascadeProps<ConfigurationSubject, Settings> {}

const extensionsContextController = createExtensionsContextController()

/** A fallback configuration subject that can be constructed synchronously at initialization time. */
const CLIENT_SUBJECT: Pick<GQL.IConfigurationSubject, 'id' | 'viewerCanAdminister'> = {
    id: 'Client',
    viewerCanAdminister: true,
}

export class CXPExtensionRegistry extends React.Component<OptionsPageProps, OptionsPageState> {
    public state: OptionsPageState = {
        configurationCascade: { subjects: [], merged: {} },
    }

    private subscriptions = new Subscription()

    public componentDidMount(): void {
        this.subscriptions.add(
            extensionsContextController.context.configurationCascade.subscribe(
                configurationCascade => this.setState({ configurationCascade }),
                err => console.error(err)
            )
        )
    }

    public render(): JSX.Element {
        return (
            <>
                <ExtensionsList
                    {...this.props}
                    subject={CLIENT_SUBJECT}
                    configurationCascade={this.state.configurationCascade}
                    extensions={extensionsContextController}
                />
                <BrowserSettingsEditor />
            </>
        )
    }
}
