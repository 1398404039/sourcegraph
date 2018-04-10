import CloseIcon from '@sourcegraph/icons/lib/Close'
import EmoticonIcon from '@sourcegraph/icons/lib/Emoticon'
import EmoticonSadIcon from '@sourcegraph/icons/lib/EmoticonSad'
import TwitterIcon from '@sourcegraph/icons/lib/Twitter'
import * as React from 'react'
import { fromEvent } from 'rxjs/observable/fromEvent'
import { filter } from 'rxjs/operators/filter'
import { Subscription } from 'rxjs/Subscription'
import { eventLogger } from '../tracking/eventLogger'
import { pluralize } from '../util/strings'

interface Props {
    user: GQL.IUser | null
    onDismiss: () => void
}

interface State {
    experience?: 'good' | 'bad'
    description: string
    isFocused: boolean
}

const TWITTER_URL = 'https://twitter.com/intent/tweet?'
const MAX_CHARACTERS = 280 - ' 😄 #UseTheSource via @srcgraph'.length
export class TwitterFeedbackForm extends React.Component<Props, State> {
    private subscriptions = new Subscription()

    constructor(props: Props) {
        super(props)

        this.state = {
            description: '',
            isFocused: false,
        }
    }

    // hide Twitter feedback box if escape key is pressed and text field isn't focused
    public componentDidMount(): void {
        this.subscriptions.add(
            fromEvent<KeyboardEvent>(window, 'keydown')
                .pipe(filter(event => !this.state.isFocused && event.key === 'Escape'))
                .subscribe(() => this.props.onDismiss())
        )
    }

    public componentWillUnmount(): void {
        this.subscriptions.unsubscribe()
    }

    public render(): JSX.Element {
        const title = 'Tweet us your feedback'
        const submitLabel = 'Tweet us'

        return (
            <form className="twitter-feedback-form theme-light" onSubmit={this.handleSubmit}>
                <button
                    type="reset"
                    className="btn btn-icon twitter-feedback-form__close"
                    onClick={this.props.onDismiss}
                >
                    <CloseIcon />
                </button>
                <div className="twitter-feedback-form__contents">
                    <h2 className="twitter-feedback-form__title">{title}</h2>
                    <div>
                        <label>How was your experience?</label>
                        <div className="twitter-feedback-form__experience">
                            <button
                                type="button"
                                className={
                                    'btn btn-icon btn-lg twitter-feedback-form__emoticon' +
                                    (this.state.experience === 'good' ? ' twitter-feedback-form__emoticon--happy' : '')
                                }
                                onClick={this.saveGoodExperience}
                            >
                                <EmoticonIcon />
                            </button>
                            <button
                                type="button"
                                className={
                                    'btn btn-icon btn-lg twitter-feedback-form__emoticon' +
                                    (this.state.experience === 'bad' ? ' twitter-feedback-form__emoticon--sad' : '')
                                }
                                onClick={this.saveBadExperience}
                            >
                                <EmoticonSadIcon />
                            </button>
                        </div>
                    </div>
                    <div className="form-group">
                        <label>Tell us why?</label>{' '}
                        <small>
                            {MAX_CHARACTERS - this.state.description.length}{' '}
                            {pluralize('characters', MAX_CHARACTERS - this.state.description.length)} left
                        </small>
                        <textarea
                            name="description"
                            id="description"
                            className="form-control "
                            placeholder="Sourcegraph code search is great #UseTheSource via @srcgraph"
                            onChange={this.handleDescriptionChange}
                            value={this.state.description}
                            required={true}
                            maxLength={MAX_CHARACTERS}
                            autoFocus={true}
                            onFocus={this.handleInputFocus}
                            onBlur={this.handleInputBlur}
                        />
                    </div>
                    <div>
                        <button type="submit" className="btn btn-primary btn-md">
                            <TwitterIcon className="icon icon-inline twitter-feedback-form__twitter-icon" />{' '}
                            {submitLabel}
                        </button>{' '}
                        <button type="reset" className="btn btn-secondary" onClick={this.props.onDismiss}>
                            Cancel
                        </button>
                    </div>
                </div>
            </form>
        )
    }

    /**
     * Tells if the query is unsupported for sending notifications.
     */
    private handleSubmit = (event: React.FormEvent<HTMLFormElement>) => {
        event.preventDefault()

        const url = new URL(TWITTER_URL)
        url.searchParams.set(
            'text',
            this.state.description +
                (this.state.experience === undefined || this.state.experience === 'good' ? ' 😄' : ' 😞') +
                '#UseTheSource via @srcgraph'
        )
        window.open(url.href)
        eventLogger.log('TwitterFeedbackButtonClicked', {
            user: this.props.user,
            experience: this.state.experience ? { experience: 'good' } : { experience: 'bad' },
        })

        this.props.onDismiss()
    }
    /**
     * Keeps track of text field focus to enable/disable box closing via escape key
     */
    private handleInputFocus = (event: React.FocusEvent<HTMLTextAreaElement>) => {
        this.setState({ isFocused: true })
    }

    private handleInputBlur = (event: React.FocusEvent<HTMLTextAreaElement>) => {
        this.setState({ isFocused: false })
    }

    private saveGoodExperience = (): void => {
        this.setState({ experience: 'good' })
    }

    private saveBadExperience = (): void => {
        this.setState({ experience: 'bad' })
    }
    /**
     * Handles description change by updating the component's state
     */
    private handleDescriptionChange = (event: React.FocusEvent<HTMLTextAreaElement>): void => {
        this.setState({ description: event.currentTarget.value })
    }
}
