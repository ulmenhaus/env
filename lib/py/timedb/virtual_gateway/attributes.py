import collections
import json

from timedb import pks, schema
from timedb.virtual_gateway import common

from jql import jql_pb2, jql_pb2_grpc


class AttributesBackend(jql_pb2_grpc.JQLServicer):

    def __init__(self, client):
        super().__init__()
        self.client = client

    def ListRows(self, request, context):
        selected_target = common.selected_target(request)
        if not selected_target:
            return common.possible_targets(self.client, request,
                                           'vt.attributes')

        attributes = self._query_attributes(selected_target)
        grouped, groupings = common.apply_grouping(attributes.values(),
                                                   request)
        max_lens = common.gather_max_lens(grouped, [])
        filtered, all_count = common.apply_request_parameters(grouped, request)
        foreign_fields = common.foreign_fields(filtered)
        final = common.convert_foreign_fields(filtered, foreign_fields)
        fields = sorted(set().union(*(final)) - {"-> Item"})
        return jql_pb2.ListRowsResponse(
            table='vt.attributes',
            columns=[
                jql_pb2.Column(name=field,
                               type=_type_of(field, foreign_fields),
                               max_length=max_lens[field],
                               primary=field == '_pk') for field in fields
            ],
            rows=[
                jql_pb2.Row(entries=[
                    jql_pb2.Entry(
                        formatted=common.present_attrs(relative[field]))
                    for field in fields
                ]) for relative in final
            ],
            total=all_count,
            all=len(attributes),
            groupings=groupings,
        )

    def _query_attributes(self, selected_target):
        requires = jql_pb2.Filter(
            column=schema.Fields.Arg0,
            equal_match=jql_pb2.EqualMatch(value=selected_target))
        rel_request = jql_pb2.ListRowsRequest(
            table=schema.Tables.Assertions,
            conditions=[
                jql_pb2.Condition(requires=[requires]),
            ],
        )
        assertions = self.client.ListRows(rel_request)
        cmap = {c.name: i for i, c in enumerate(assertions.columns)}
        primary = common.get_primary(assertions)
        attributes = {}
        for row in assertions.rows:
            pk = row.entries[primary].formatted
            attributes[pk] = {
                "_pk": [pk],
                "Relation":
                [row.entries[cmap[schema.Fields.Relation]].formatted],
                "Value": [row.entries[cmap[schema.Fields.Arg1]].formatted],
                "-> Item":
                [selected_target],  # added to ensure the filter still matches
                "_Item": [
                    selected_target
                ],  # added so that duplicated items still show up in the filter
                "Order": [row.entries[cmap[schema.Fields.Order]].formatted],
            }
        return attributes

    def WriteRow(self, request, context):
        mapping = {
            schema.Fields.Order: "Order",
            schema.Fields.Arg1: "Value",
            schema.Fields.Arg0: "_Item",
            schema.Fields.Relation: "Relation",
        }
        fields = {
            k: request.fields[v]
            for k, v in mapping.items() if v in request.fields
        }
        update = jql_pb2.WriteRowRequest(
            table=schema.Tables.Assertions,
            pk=request.pk,
            fields=fields,
            insert_only=request.insert_only,
            update_only=request.update_only,
        )
        return self.client.WriteRow(update)

    def DeleteRow(self, request, context):
        return self.client.DeleteRow(
            jql_pb2.DeleteRowRequest(
                table=schema.Tables.Assertions,
                pk=request.pk,
            ))


def is_verb(attribute):
    return attribute.endswith("es")


def _type_of(field, foreign):
    if field == "Display Name":
        return jql_pb2.EntryType.POLYFOREIGN
    if field in foreign:
        return jql_pb2.EntryType.POLYFOREIGN
    return jql_pb2.EntryType.STRING
