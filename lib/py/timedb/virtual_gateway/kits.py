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
        kits = self._query_kits()
        grouped, groupings = common.apply_grouping(kits.values(),
                                                   request)
        max_lens = common.gather_max_lens(grouped, [])
        filtered, all_count = common.apply_request_parameters(grouped, request)
        fields = sorted(set().union(*(kits.values())))
        foreign_fields = common.foreign_fields(filtered)
        final = common.convert_foreign_fields(filtered, foreign_fields)
        return jql_pb2.ListRowsResponse(
            table='vt.kits',
            columns=[
                jql_pb2.Column(name=field,
                               type=_type_of(field, foreign_fields),
                               max_length=max_lens.get(field, 0),
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
            all=len(kits),
            groupings=groupings,
        )

    def _query_kits(self):
        assns = self.client.ListRows(jql_pb2.ListRowsRequest(
            table=schema.Tables.Assertions,
            conditions=[
                jql_pb2.Condition(requires=[
                    jql_pb2.Filter(column=schema.Fields.Relation,
                                   equal_match=jql_pb2.EqualMatch(value=".KitDomain")),
                ], ),
            ],
        ))
        assn_cmap = {c.name: i for i, c in enumerate(assns.columns)}
        rows = {}
        for row in assns.rows:
            table, kit = row.entries[assn_cmap[schema.Fields.Arg0]].formatted.split(" ", 1)
            if table != schema.Tables.Nouns:
                continue
            rows[kit] = {
                "_pk": [kit],
                "Action": ["Warm-up"],
                "Direct": [kit],
                "Motivation": ["Preparation"],
                "Source": [""],
                "Towards": [""],
                "Domain": [row.entries[assn_cmap[schema.Fields.Arg1]].formatted],
            }
        return rows


    def GetRow(self, request, context):
        resp = self.ListRows(jql_pb2.ListRowsRequest(), context)
        primary = common.get_primary(resp)
        for row in resp.rows:
            if row.entries[primary].formatted == request.pk:
                return jql_pb2.GetRowResponse(
                    table='vt.kits',
                    columns = resp.columns,
                    row=row,
                )
        raise ValueError(request.pk)

def _type_of(field, foreign):
    if field in foreign:
        return jql_pb2.EntryType.POLYFOREIGN
    # TODO make the object field a foreign field to nouns
    return jql_pb2.EntryType.STRING
