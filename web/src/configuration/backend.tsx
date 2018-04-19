import { Observable } from 'rxjs/Observable'
import { map } from 'rxjs/operators/map'
import { mergeMap } from 'rxjs/operators/mergeMap'
import { take } from 'rxjs/operators/take'
import { gql, GraphQLDocument, mutateGraphQL, MutationResult } from '../backend/graphql'
import * as GQL from '../backend/graphqlschema'
import { configurationCascade } from '../settings/configuration'
import { refreshConfiguration } from '../user/settings/backend'
import { createAggregateError } from '../util/errors'

/**
 * Updates the configuration for a subject to the value produced by the given update function.
 *
 * @param subject The subject whose configuration to update.
 * @param update Called on a copy of the old (current) config to produce the new config
 */
export function updateConfiguration(
    subject: GQL.ConfigurationSubject | GQL.IConfigurationSubject | { id: GQL.ID },
    input: GQL.IUpdateConfigurationInput
): Observable<void> {
    const subjectID = subject.id
    if (!subjectID) {
        throw new Error('subject has no id')
    }
    return configurationCascade.pipe(
        take(1),
        mergeMap(config => {
            const subjectConfig = config.subjects.find(s => s.id === subjectID)
            if (!subjectConfig) {
                throw new Error(`no configuration subject: ${subjectID}`)
            }
            const lastID = subjectConfig.latestSettings ? subjectConfig.latestSettings.id : null
            return doUpdateConfiguration({ subject: subjectID, lastID }, input)
        })
    )
}

/**
 * Sends a GraphQL mutation to update configuration.
 */
function doUpdateConfiguration(
    configuration: GQL.IConfigurationMutationGroupInput,
    input: GQL.IUpdateConfigurationInput
): Observable<void> {
    return mutateGraphQL(
        gql`
            mutation UpdateConfiguration(
                $configurationInput: ConfigurationMutationGroupInput!
                $updateInput: UpdateConfigurationInput
            ) {
                configurationMutation(input: $configurationInput) {
                    updateConfiguration(input: $updateInput) {
                        alwaysNil
                    }
                }
            }
        `,
        { configurationInput: configuration, updateInput: input }
    ).pipe(
        mergeMap(({ data, errors }) => {
            if (!data || !data.configurationMutation || data.configurationMutation.updateConfiguration) {
                throw createAggregateError(errors)
            }
            return refreshConfiguration()
        })
    )
}

/**
 * Runs a GraphQL mutation that includes configuration mutations, populating the variables object
 * with the lastID and subject for the configuration mutation.
 *
 * @param subject The subject whose configuration to update.
 * @param mutation The GraphQL mutation.
 * @param variables The GraphQL mutation's variables.
 */
export function mutateConfigurationGraphQL(
    subject: GQL.ConfigurationSubject | GQL.IConfigurationSubject | { id: GQL.ID },
    mutation: GraphQLDocument,
    variables: any = {}
): Observable<MutationResult> {
    const subjectID = subject.id
    if (!subjectID) {
        throw new Error('subject has no id')
    }
    return configurationCascade.pipe(
        take(1),
        mergeMap(config => {
            const subjectConfig = config.subjects.find(s => s.id === subjectID)
            if (!subjectConfig) {
                throw new Error(`no configuration subject: ${subjectID}`)
            }
            const lastID = subjectConfig.latestSettings ? subjectConfig.latestSettings.id : null

            return mutateGraphQL(mutation, { ...variables, subject: subjectID, lastID })
        }),
        map(result => {
            refreshConfiguration().subscribe()
            return result
        })
    )
}
