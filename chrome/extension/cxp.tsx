// We want to polyfill first.
import '../../app/util/polyfill'

import * as React from 'react'
import { Button, FormGroup, Input, Label } from 'reactstrap'
import { Subscription } from 'rxjs'
import storage from '../../extension/storage'

interface State {
    clientSettings: string
}

export class BrowserSettingsEditor extends React.Component<{}, State> {
    private subscriptions = new Subscription()

    constructor(props) {
        super(props)
        this.state = {
            clientSettings: 'Loading...',
        }
    }

    public componentDidMount(): void {
        this.subscriptions.add(
            storage.observeSync('clientSettings').subscribe(clientSettings => {
                this.setState(() => ({ clientSettings }))
            })
        )
    }

    public componentWillUnmount(): void {
        this.subscriptions.unsubscribe()
    }

    private saveLocalSettings = () => {
        storage.setSync({ clientSettings: this.state.clientSettings })
    }

    private onSettingsChanged = event => {
        const value = event.target.value
        this.setState(() => ({ clientSettings: value }))
    }

    public render(): JSX.Element | null {
        return (
            <div className="options__section">
                <div className="options__section-header">Client settings</div>
                <div className="options__section-contents">
                    <FormGroup>
                        <Label className="options__input">
                            <Input
                                className="options__input-textarea"
                                type="textarea"
                                value={this.state.clientSettings}
                                onChange={this.onSettingsChanged}
                                autoComplete="off"
                                autoCorrect="off"
                                autoCapitalize="off"
                                spellCheck={false}
                            />
                        </Label>
                        <Button className="options__cta" color="primary" onClick={this.saveLocalSettings}>
                            Save
                        </Button>
                    </FormGroup>
                </div>
            </div>
        )
    }
}
