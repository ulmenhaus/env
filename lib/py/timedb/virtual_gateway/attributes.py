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
        return common.list_rows('vt.attributes', attributes, request)

    def _create_row_mapping(self, pk, attrs):
        mapping = {
            "_pk": [pk],
        }
        mapping.update(attrs)
        return mapping

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
            attrs = {
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
            pk = common.encode_pk(row.entries[primary].formatted, attrs)
            attributes[pk] = self._create_row_mapping(pk, attrs)
        return attributes

    def GetRow(self, request, context):
        _assn_pk, attrs = common.decode_pk(request.pk)
        mapping = self._create_row_mapping(request.pk, attrs)
        return common.return_row('vt.attributes', mapping)

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
        pk, _ = common.decode_pk(request.pk)
        update = jql_pb2.WriteRowRequest(
            table=schema.Tables.Assertions,
            pk=pk,
            fields=fields,
            insert_only=request.insert_only,
            update_only=request.update_only,
        )
        return self.client.WriteRow(update)

    def DeleteRow(self, request, context):
        pk, _ = common.decode_pk(request.pk)
        return self.client.DeleteRow(
            jql_pb2.DeleteRowRequest(
                table=schema.Tables.Assertions,
                pk=pk,
            ))


def is_verb(attribute):
    return attribute.endswith("es")
