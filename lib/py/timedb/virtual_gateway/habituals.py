import json

from datetime import datetime

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
        primary, = [
            i for i, c in enumerate(habituals_response.columns) if c.primary
        ]
        habituals_cmap = {c.name: i for i, c in enumerate(habituals_response.columns)}
        noun_pks = [
            row.entries[primary].formatted for row in habituals_response.rows
        ]
        # Populate all relevant fields for the given nouns
        fields = ["Cadence"]
        noun_to_habitual, assn_pks = common.get_fields_for_items(
            self.client, schema.Tables.Nouns, noun_pks, fields)
        for row in habituals_response.rows:
            noun_pk = row.entries[primary].formatted
            noun_to_habitual[noun_pk]["Parent"] = [row.entries[habituals_cmap[
                schema.Fields.Parent]].formatted]
            noun_to_habitual[noun_pk]["Habitual"] = [noun_pk]
            noun_to_habitual[noun_pk]["_pk"] = [_encode_pk(noun_pk, assn_pks[noun_pk])]

        fields = ["Parent", "Habitual"] + fields + ["Days Since", "Days Until", "_pk"]
        # Populate "Days Since" as the number of days since a task has featured this
        # habitual
        #
        # TODO maybe also worth populating based on any tasks with field values that are
        # this noun
        for noun_pk, days_since in self._days_since(noun_pks).items():
            habitual = noun_to_habitual[noun_pk]
            habitual["Days Since"] = [str(days_since).zfill(4)]
            if "Cadence" in habitual:
                days_until = int(habitual["Cadence"][0].split(" ")[0]) - days_since
                if days_until > 0:
                    days_until_s = "+" + str(days_until).zfill(4)
                else:
                    days_until_s = str(days_until).zfill(5)
                habitual["Days Until"] = [days_until_s]
        # apply sorting, filtering, and limiting -- this portion can be made generic
        habituals, all_count = common.apply_request_parameters(
            noun_to_habitual.values(), request)
        return jql_pb2.ListRowsResponse(
            table='vt.habituals',
            columns=[
                jql_pb2.Column(name=field,
                               max_length=30,
                               type=_type_of(field),
                               foreign_table='nouns' if field == 'Habitual' else '',
                               values=VALUES.get(field, []),
                               primary=field == '_pk') for field in fields
            ],
            rows=[
                jql_pb2.Row(entries=[
                    jql_pb2.Entry(formatted=common.present_attrs(habitual[field])) for field in fields
                ]) for habitual in habituals
            ],
            total=all_count,
            all=len(habituals_response.rows),
        )

    def IncrementEntry(self, request, context):
        noun_pk, pk_map = _decode_pk(request.pk)
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
            next_value = values[(current_index + request.amount) %
                                len(values)]
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

    def _days_since(self, noun_pks):
        # TODO once we support multiple conditions this can be done
        # in one query. Other nicities that would make this UX nicer
        #
        # 1. Group by direct/indirect and limit 1 so we get the exact
        #    entry we want
        # 2. Provide a format string for the date entry so we can
        #    convert to UNIX timestamp on the server side
        # 3. Filter columns to just the relevant ones
        rows = []
        ret = {}
        for column in [schema.Fields.Direct, schema.Fields.Indirect]:
            tasks_request = jql_pb2.ListRowsRequest(
                table=schema.Tables.Tasks,
                conditions=[
                    jql_pb2.Condition(requires=[
                        jql_pb2.Filter(
                            column=column,
                            in_match=jql_pb2.InMatch(
                                values=noun_pks),
                        ),
                    ]),
                ],
            )
            tasks_response = self.client.ListRows(tasks_request)
            rows += tasks_response.rows
        tasks_cmap = {c.name: i for i, c in enumerate(tasks_response.columns)}
        noun_pks_set = set(noun_pks)
        for row in rows:
            start_formatted = row.entries[tasks_cmap[schema.Fields.ParamStart]].formatted
            days_since = (datetime.now() - datetime.strptime(start_formatted, "%d %b %Y")).days
            for obj in [schema.Fields.Direct, schema.Fields.Indirect]:
                value = row.entries[tasks_cmap[obj]].formatted
                if value not in noun_pks_set:
                    continue
                if (value not in ret) or (ret[value] > days_since):
                    ret[value] = days_since
                
        return ret

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
    if field == 'Habitual':
        return jql_pb2.EntryType.FOREIGN
    elif field in VALUES:
        return jql_pb2.EntryType.ENUM
    return jql_pb2.EntryType.STRING

def _encode_pk(noun_pk, assn_pks):
    return "\t".join([noun_pk, json.dumps(assn_pks)])

def _decode_pk(pk):
    noun_pk, assn_pks = pk.split("\t")
    return noun_pk, json.loads(assn_pks)
