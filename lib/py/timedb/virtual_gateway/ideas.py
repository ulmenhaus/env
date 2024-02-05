import json

from timedb import schema
from timedb.virtual_gateway import common

from jql import jql_pb2, jql_pb2_grpc

VALUES = {
    "Cost": [
        "1 O(hours)",
        "2 O(days)",
        "3 O(weeks)",
        "4 O(months)",
        "5 O(quarters)",
        "6 O(years)",
    ],
    "SoB": [
        "Time Efficiency",  # New investments in tools that create efficiency wins
        "Simplicity/Consistency",  # Improvements of existing tools that create efficiency wins
        "Joissance",  # Diverse, rich, and pleasurable (in particular sensory) experiences
        "Achievement",  # Challenge you to prove your mettle, gives external and internal validaton of competence -> security, feeling of accomplishment
        "Fulfillment",  # Make you whole, content, feel like you are elevating yourself/humanity
        "Self Expression",  # Aesthetic/Creative fulfillment
    ],
    "RoI": [
        "1 Very Low",
        "2 Low",
        "3 Medium",
        "4 High",
        "5 Very High",
    ],
}


class IdeasBackend(jql_pb2_grpc.JQLServicer):

    def __init__(self, client):
        super().__init__()
        self.client = client

    def ListRows(self, request, context):
        ideas_request = jql_pb2.ListRowsRequest(
            table=schema.Tables.Nouns,
            conditions=[
                jql_pb2.Condition(requires=[
                    jql_pb2.Filter(
                        column=schema.Fields.Status,
                        equal_match=jql_pb2.EqualMatch(
                            value=schema.Values.StatusIdea),
                    ),
                ]),
            ],
        )
        ideas_response = self.client.ListRows(ideas_request)
        primary, = [
            i for i, c in enumerate(ideas_response.columns) if c.primary
        ]
        ideas_cmap = {c.name: i for i, c in enumerate(ideas_response.columns)}
        noun_pks = [
            row.entries[primary].formatted for row in ideas_response.rows
        ]
        # Populate all relevant fields for the given nouns
        fields = ["Domain", "Parent", "Cost", "SoB", "RoI", "Idea", "_pk"]
        noun_to_idea, assn_pks = common.get_fields_for_items(
            self.client, schema.Tables.Nouns, noun_pks, fields)
        for row in ideas_response.rows:
            noun_pk = row.entries[primary].formatted
            noun_to_idea[noun_pk]["Parent"] = [
                row.entries[ideas_cmap[schema.Fields.Parent]].formatted
            ]
            noun_to_idea[noun_pk]["Idea"] = [noun_pk]
            noun_to_idea[noun_pk]["_pk"] = [_encode_pk(noun_pk, assn_pks[noun_pk])]

        parent_pks = sorted(
            {idea["Parent"][0]
             for idea in noun_to_idea.values()})
        domains, _ = common.get_fields_for_items(self.client,
                                                 schema.Tables.Nouns,
                                                 parent_pks, ["Domain"])
        for idea in noun_to_idea.values():
            idea["Domain"] = domains[idea["Parent"][0]]["Domain"]
        # apply sorting, filtering, and limiting -- this portion can be made generic
        ideas, all_count = common.apply_request_parameters(
            noun_to_idea.values(), request)
        return jql_pb2.ListRowsResponse(
            table='vt.ideas',
            columns=[
                jql_pb2.Column(name=field,
                               max_length=30,
                               type=_type_of(field),
                               foreign_table='nouns' if field == 'Idea' else '',
                               values=VALUES.get(field, []),
                               primary=field == '_pk') for field in fields
            ],
            rows=[
                jql_pb2.Row(entries=[
                    jql_pb2.Entry(
                        formatted=common.present_attrs(idea[field]),
                    ) for field in fields
                ]) for idea in ideas
            ],
            total=all_count,
            all=len(ideas_response.rows),
        )

    def IncrementEntry(self, request, context):
        noun_pk, pk_map = _decode_pk(request.pk)
        if request.column == 'Idea':
            # If it's the habitual itself we're incrementing/decrementing that corresponds
            # to the status
            next_status = schema.Values.StatusSatisfied if request.amount > 0 else schema.Values.StatusRevisit
            request = jql_pb2.WriteRowRequest(
                table=schema.Tables.Nouns,
                pk=noun_pk,
                fields={schema.Fields.Status: next_status},
                update_only=True,
            )
            self.client.WriteRow(request)
            return jql_pb2.IncrementEntryResponse()
        elif request.column in pk_map:
            assn_pk, current = pk_map[request.column]
            values = VALUES[request.column]
            current_index = values.index(
                current) if current in values else -request.amount
            next_value = values[(current_index + request.amount) % len(values)]
            request = jql_pb2.WriteRowRequest(
                table=schema.Tables.Assertions,
                pk=assn_pk,
                fields={schema.Fields.Arg1: next_value},
                update_only=True,
            )
            self.client.WriteRow(request)
            return jql_pb2.IncrementEntryResponse()
        elif request.column in VALUES:
            value = VALUES[request.column][0]
            request = jql_pb2.WriteRowRequest(
                table=schema.Tables.Assertions,
                pk=str((f".{request.column}", noun_pk, "0000")),
                fields={
                    schema.Fields.Relation: f".{request.column}",
                    schema.Fields.Arg0: f"nouns {noun_pk}",
                    schema.Fields.Arg1: value,
                },
                insert_only=True,
            )
            self.client.WriteRow(request)
            return jql_pb2.IncrementEntryResponse()
        else:
            raise ValueError("Unknown column", request.column)

    def WriteRow(self, request, context):
        noun_pk, pk_map = _decode_pk(request.pk)
        for field, value in request.fields.items():
            if field in pk_map:
                assn_pk, current = pk_map[field]
                request = jql_pb2.WriteRowRequest(
                    table=schema.Tables.Assertions,
                    pk=assn_pk,
                    fields={schema.Fields.Arg1: value},
                    update_only=True,
                )
                self.client.WriteRow(request)
            elif field in VALUES:
                request = jql_pb2.WriteRowRequest(
                    table=schema.Tables.Assertions,
                    pk=str((f".{field}", noun_pk, "0000")),
                    fields={
                        schema.Fields.Relation: f".{field}",
                        schema.Fields.Arg0: f"nouns {noun_pk}",
                        schema.Fields.Arg1: value,
                    },
                    insert_only=True,
                )
                self.client.WriteRow(request)
            else:
                raise ValueError("Unknown column", request.column)
        return jql_pb2.WriteRowResponse()


def _type_of(field):
    if field == 'Idea':
        return jql_pb2.EntryType.FOREIGN
    elif field in VALUES:
        return jql_pb2.EntryType.ENUM
    return jql_pb2.EntryType.STRING

def _encode_pk(noun_pk, assn_pks):
    return "\t".join([noun_pk, json.dumps(assn_pks)])

def _decode_pk(pk):
    noun_pk, assn_pks = pk.split("\t")
    return noun_pk, json.loads(assn_pks)
