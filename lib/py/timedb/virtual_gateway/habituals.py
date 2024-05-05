import json

from timedb import schema
from timedb.virtual_gateway import common

from jql import jql_pb2, jql_pb2_grpc

VALUES = {
    "Cadence": [
        "001 Daily",
        "007 Weekly",
        "030 Monthly",
        "090 Quarterly",
        "365 Yearly",
        "1825 Quinquennial",
    ],
}


class HabitualsBackend(jql_pb2_grpc.JQLServicer):

    def __init__(self, client):
        super().__init__()
        self.client = client

    def ListRows(self, request, context):
        habituals_request = jql_pb2.ListRowsRequest(
            table=schema.Tables.Nouns,
            conditions=[
                jql_pb2.Condition(requires=[
                    jql_pb2.Filter(
                        column=schema.Fields.Status,
                        equal_match=jql_pb2.EqualMatch(
                            value=schema.Values.StatusHabitual),
                    ),
                ]),
            ],
        )
        habituals_response = self.client.ListRows(habituals_request)
        primary, cmap = common.list_rows_meta(habituals_response)
        noun_pks = sorted(
            set([
                row.entries[primary].formatted
                if row.entries[cmap[schema.Fields.Modifier]].formatted !=
                common.ALIAS_MODIFIER else
                row.entries[cmap[schema.Fields.Description]].formatted
                for row in habituals_response.rows
            ]))
        habitual2info = common.get_habitual_info(self.client, noun_pks)
        parents = {
            row.entries[primary].formatted:
            row.entries[cmap[schema.Fields.Parent]].formatted
            for row in habituals_response.rows
        }
        entries = {}
        for habitual, info in habitual2info.items():
            pk = common.encode_pk(habitual, info.cadence_pk)
            entries[pk] = {
                "Parent": [parents[habitual]],
                "Habitual": [f"@timedb:{habitual}:"],
                "Days Since": [info.days_since],
                "Days Until": [info.days_until],
                "_pk": [pk],
                "Cadence": [info.cadence],
            }
        return common.list_rows('vt.habituals', entries, request, VALUES)

    def IncrementEntry(self, request, context):
        noun_pk, pk_map = common.decode_pk(request.pk)
        if request.column == 'Habitual':
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
            current_index = values.index(current) if current in values else 0
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
