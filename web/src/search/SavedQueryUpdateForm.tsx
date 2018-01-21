import * as React from 'react'
import { Observable } from 'rxjs/Observable'
import { concat } from 'rxjs/operators/concat'
import { mergeMap } from 'rxjs/operators/mergeMap'
import { refreshConfiguration } from '../user/settings/backend'
import { createSavedQuery, deleteSavedQuery, updateSavedQuery } from './backend'
import { SavedQueryFields, SavedQueryForm } from './SavedQueryForm'

interface Props {
    savedQuery: GQL.ISavedQuery
    onDidUpdate: () => void
    onDidCancel: () => void
}

export const SavedQueryUpdateForm: React.StatelessComponent<Props> = props => (
    <SavedQueryForm
        defaultValues={{
            description: props.savedQuery.description,
            query: props.savedQuery.query.query,
            subject: props.savedQuery.subject.id,
            showOnHomepage: props.savedQuery.showOnHomepage,
        }}
        onDidCommit={props.onDidUpdate}
        onDidCancel={props.onDidCancel}
        submitLabel="Save"
        // tslint:disable-next-line:jsx-no-lambda
        onSubmit={fields => updateSavedQueryFromForm(props, fields)}
    />
)

function updateSavedQueryFromForm(props: Props, fields: SavedQueryFields): Observable<any> {
    // If the subject changed, we need to create it on the new subject and
    // delete it on the old subject.
    if (props.savedQuery.subject.id !== fields.subject) {
        return createSavedQuery({ id: fields.subject }, fields.description, fields.query, fields.showOnHomepage).pipe(
            mergeMap(() => deleteSavedQuery(props.savedQuery.subject, props.savedQuery.id)),
            mergeMap(() => refreshConfiguration().pipe(concat([null])))
        )
    }

    // Otherwise, it's just a simple update.
    return updateSavedQuery(
        props.savedQuery.subject,
        props.savedQuery.id,
        fields.description,
        fields.query,
        fields.showOnHomepage
    )
}
