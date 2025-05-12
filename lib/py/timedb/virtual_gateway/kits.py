import collections
import json

from timedb import pks, schema
from timedb.virtual_gateway import common

from jql import jql_pb2, jql_pb2_grpc


class KitsBackend(jql_pb2_grpc.JQLServicer):

    def __init__(self, client):
        super().__init__()
        self.client = client

    def ListRows(self, request, context):
        selected_parent = _extract_selected_parent(request)
        kits = self._query_kits(selected_parent)
        return common.list_rows('vt.kits', kits, request)

    def _query_kits(self, selected_parent):
        assns = self.client.ListRows(
            jql_pb2.ListRowsRequest(
                table=schema.Tables.Assertions,
                conditions=[
                    jql_pb2.Condition(requires=[
                        jql_pb2.Filter(column=schema.Fields.Relation,
                                       equal_match=jql_pb2.EqualMatch(
                                           value=".KitDomain")),
                    ], ),
                ],
            ))
        assn_cmap = {c.name: i for i, c in enumerate(assns.columns)}
        kits = {}
        for row in assns.rows:
            table, kit = row.entries[assn_cmap[
                schema.Fields.Arg0]].formatted.split(" ", 1)
            if table != schema.Tables.Nouns:
                continue
            kits[kit] = row
        kit2info = common.get_timing_info(self.client, kits.keys())
        rows = {}
        for kit, row in kits.items():
            if kit in kit2info and len(kit2info[kit].active_actions) > 0:
                continue
            pk = _encode_pk(kit, selected_parent)
            rows[pk] = {
                "_pk": [pk],
                "Action": ["Warm-up"],
                "Days Since": [""],
                "Days Until": [""],
                "Direct": [kit],
                "Motivation": ["Preparation"],
                "Source": [""],
                "Towards": [""],
                "Domain":
                [row.entries[assn_cmap[schema.Fields.Arg1]].formatted],
                "Parent": [selected_parent],
            }
            if kit in kit2info:
                info = kit2info[kit]
                rows[pk]['Days Since'] = [info.days_since]
                rows[pk]['Days Until'] = [info.days_until]
        return rows

    def GetRow(self, request, context):
        _, parent = _decode_pk(request.pk)
        resp = self.ListRows(
            jql_pb2.ListRowsRequest(conditions=[
                jql_pb2.Condition(requires=[
                    jql_pb2.Filter(
                        column='Parent',
                        equal_match=jql_pb2.EqualMatch(value=parent)),
                ], ),
            ], ), context)
        primary = common.get_primary(resp)
        for row in resp.rows:
            if row.entries[primary].formatted == request.pk:
                return jql_pb2.GetRowResponse(
                    table='vt.kits',
                    columns=resp.columns,
                    row=row,
                )
        raise ValueError(request.pk)


def _type_of(field, foreign):
    if field == "Parent":
        return jql_pb2.EntryType.POLYFOREIGN, '', []
    # TODO make the object field a foreign field to nouns
    return jql_pb2.EntryType.STRING, '', []


def _extract_selected_parent(request):
    for condition in request.conditions:
        for f in condition.requires:
            match_type = f.WhichOneof('match')
            if match_type == "equal_match" and f.column == 'Parent':
                return f.equal_match.value
    return ""


def _encode_pk(kit, parent):
    return "\t".join([kit, parent])


def _decode_pk(pk):
    kit, parent = pk.split("\t")
    return kit, parent
