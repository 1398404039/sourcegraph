import * as React from 'react'
import { Form } from '../../components/Form'
import CloseIcon from 'mdi-react/CloseIcon'

interface Props {
    /** The key of this filter in the top-level fieldValues map. */
    mapKey: string
    value: string
    filterType: string
    editable: boolean
    onFilterEdited: (filterKey: string, value: string) => void
    onFilterDeleted: (filterKey: string) => void
    toggleFilterEditable: (filterKey: string) => void
}

export default class InteractiveModeFilterChip extends React.Component<Props> {
    private onInputUpdate = (e: React.ChangeEvent<HTMLInputElement>): void => {
        this.props.onFilterEdited(this.props.mapKey, e.target.value)
    }

    private onSubmitInput = (e: React.FormEvent<HTMLFormElement>): void => {
        e.preventDefault()
        e.stopPropagation()

        this.props.toggleFilterEditable(this.props.mapKey)
    }

    private onClickSelected = (): void => {
        this.props.toggleFilterEditable(this.props.mapKey)
    }

    private onClickDelete = (): void => {
        this.props.onFilterDeleted(this.props.mapKey)
    }

    public render(): JSX.Element | null {
        return (
            <>
                {this.props.editable ? (
                    <Form onSubmit={this.onSubmitInput}>
                        <div>
                            <input onChange={this.onInputUpdate} value={this.props.value} />
                            <div onClick={this.onClickDelete}>
                                <CloseIcon />
                            </div>
                        </div>
                    </Form>
                ) : (
                    <div className="d-flex">
                        <div onClick={this.onClickSelected}>{this.props.value}</div>
                        <div onClick={this.onClickDelete}>
                            <CloseIcon />
                        </div>
                    </div>
                )}
            </>
        )
    }
}
