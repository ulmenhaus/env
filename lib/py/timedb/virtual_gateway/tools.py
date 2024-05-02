import collections
import json

from timedb import pks, schema
from timedb.virtual_gateway import common

from jql import jql_pb2, jql_pb2_grpc


class ToolsBackend(jql_pb2_grpc.JQLServicer):

    def __init__(self, client):
        super().__init__()
        self.client = client

    def ListRows(self, request, context):
        selected_target = common.selected_target(request)
        selected_parent = _extract_selected_parent(request)
        exercises = self._query_exercises(selected_target, selected_parent)
        return common.list_rows('vt.practices', exercises, _type_of, request)

    def _query_exercises(self, selected_target, selected_parent):
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
        target2relation = {}
        for row in assertions.rows:
            target = row.entries[cmap[schema.Fields.Arg1]].formatted
            if not common.is_foreign(target):
                continue
            target_pk = common.strip_foreign(target)
            relation = [row.entries[cmap[schema.Fields.Relation]].formatted]
            target2relation[target_pk] = relation

        fields, _ = common.get_fields_for_items(self.client, schema.Tables.Nouns, list(target2relation.keys()))
        for target_pk, relation in target2relation.items():
            actions = fields.get(target_pk, {}).get("Feed.Action", []) or ["Exercise", "Ready", "Evaluate"]
            for action in actions:
                exercise = f"{action} {target_pk}"
                pk = _encode_pk(exercise, selected_parent, selected_target)
                attributes[pk] = {
                    "_pk": [pk],
                    "Relation": relation,
                    "Action": [action],
                    "Direct": [target_pk],
                    "Parent": [selected_parent],
                    "-> Item":
                    [selected_target],  # added to ensure the filter still matches
                    "Motivation": ["Preparation"],
                    "Source": [""],
                    "Towards": [""],
                    "Domain": [""],
                }
        return attributes

    def GetRow(self, request, context):
        _, parent, target = _decode_pk(request.pk)
        resp = self.ListRows(jql_pb2.ListRowsRequest(
            conditions=[
                jql_pb2.Condition(requires=[
                    jql_pb2.Filter(column='Parent',
                                   equal_match=jql_pb2.EqualMatch(value=parent)),
                    jql_pb2.Filter(column='-> Item',
                                   equal_match=jql_pb2.EqualMatch(value=target)),
                ], ),
            ],
        ), context)
        primary = common.get_primary(resp)
        for row in resp.rows:
            if row.entries[primary].formatted == request.pk:
                return jql_pb2.GetRowResponse(
                    table='vt.tools',
                    columns = resp.columns,
                    row=row,
                )
        raise ValueError(request.pk)

def _type_of(field, foreign):
    if field == "Display Name":
        return jql_pb2.EntryType.POLYFOREIGN, '', []
    if field in foreign:
        return jql_pb2.EntryType.POLYFOREIGN, '', []
    return jql_pb2.EntryType.STRING, '', []

def _extract_selected_parent(request):
    for condition in request.conditions:
        for f in condition.requires:
            match_type = f.WhichOneof('match')
            if match_type == "equal_match" and f.column == 'Parent':
                return f.equal_match.value
    return ""

def _encode_pk(exercise, parent, target):
    return "\t".join([exercise, parent, target])

def _decode_pk(pk):
    exercise, parent, target = pk.split("\t")
    return exercise, parent, target
