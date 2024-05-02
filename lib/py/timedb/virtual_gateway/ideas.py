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
    "PE": [
        "1 (Hours)",
        "2 (Days)",
        "3 (Weeks)",
        "4 (Months)",
        "5 (Quarters)",
        "6 (Years)",
    ],
    "SoB": [
        # Investment
        "Time Efficiency",  # New investments in tools that create efficiency wins
        "Risk Reduction",
        "Future Gains",
        "Simplicity/Consistency",  # Improvements of existing tools that create efficiency wins
        "Rest",
        # Happiness"
        "Joissance",  # Diverse, rich, and pleasurable (in particular sensory) experiences
        "Pleasure",
        "Eudemonic",
        # Functional Output
        "Achievement",  # Challenge you to prove your mettle, gives external and internal validaton of competence -> security, feeling of accomplishment
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
        primary = common.get_primary(ideas_response)
        ideas_cmap = {c.name: i for i, c in enumerate(ideas_response.columns)}
        noun_pks = [
            row.entries[primary].formatted for row in ideas_response.rows
        ]
        # Populate all relevant fields for the given nouns
        fields = ["Domain", "Parent", "Cost", "PE", "SoB", "RoI", "Idea", "_pk"]
        noun_to_idea, assn_pks = common.get_fields_for_items(
            self.client, schema.Tables.Nouns, noun_pks, fields)
        for row in ideas_response.rows:
            noun_pk = row.entries[primary].formatted
            idea = noun_to_idea[noun_pk]
            idea["Parent"] = [
                row.entries[ideas_cmap[schema.Fields.Parent]].formatted
            ]
            idea["Idea"] = [f"@timedb:{noun_pk}:"]
            idea["_pk"] = [common.encode_pk(noun_pk, assn_pks[noun_pk])]
            if idea["PE"] and idea["PE"][0] and idea["Cost"] and idea["Cost"][0]:
                # Entries denote orders of magnitude so determine RoI through subtraction
                idea["RoI"] = [str(int(idea["PE"][0].split(" ")[0]) - int(idea["Cost"][0].split(" ")[0]))]

        parent_pks = sorted(
            {idea["Parent"][0]
             for idea in noun_to_idea.values()})
        domains, _ = common.get_fields_for_items(self.client,
                                                 schema.Tables.Nouns,
                                                 parent_pks, ["Domain"])
        for idea in noun_to_idea.values():
            idea["Domain"] = domains[idea["Parent"][0]]["Domain"]
        return common.list_rows('vt.ideas', noun_to_idea, request)

    def IncrementEntry(self, request, context):
        noun_pk, pk_map = common.decode_pk(request.pk)
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
        noun_pk, pk_map = common.decode_pk(request.pk)
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
