import collections
import json

from timedb import pks, schema
from timedb.virtual_gateway import common

from jql import jql_pb2, jql_pb2_grpc


class PracticesBackend(jql_pb2_grpc.JQLServicer):

    def __init__(self, client):
        super().__init__()
        self.client = client

    def ListRows(self, request, context):
        feeds = self._query_feeds()
        actionable = self._query_actionable_children(feeds)
        grouped, groupings = common.apply_grouping(actionable.values(),
                                                   request)
        max_lens = common.gather_max_lens(grouped, [])
        filtered, all_count = common.apply_request_parameters(grouped, request)
        fields = sorted(set().union(*(actionable.values())))
        foreign_fields = common.foreign_fields(filtered)
        final = common.convert_foreign_fields(filtered, foreign_fields)
        return jql_pb2.ListRowsResponse(
            table='vt.practices',
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
            all=len(actionable),
            groupings=groupings,
        )

    def _query_feeds(self):
        nouns_request = jql_pb2.ListRowsRequest(
            table=schema.Tables.Nouns,
            conditions=[
                jql_pb2.Condition(requires=[
                    jql_pb2.Filter(column=schema.Fields.Feed,
                                   negated=True,
                                   equal_match=jql_pb2.EqualMatch(value="", )),
                ], ),
            ],
        )
        nouns = self.client.ListRows(nouns_request)
        primary = common.get_primary(nouns)
        return {row.entries[primary].formatted for row in nouns.rows}

    def _query_actionable_children(self, feeds):
        feed_attrs, _ = common.get_fields_for_items(self.client, schema.Tables.Nouns, feeds)
        action_map = {
            schema.Values.StatusExploring: "Explore",
            schema.Values.StatusPlanning: "Plan",
            schema.Values.StatusImplementing: "Implement",
            schema.Values.StatusHabitual: "Implement",
            schema.Values.StatusSatisfied: "Implement",
            schema.Values.StatusRevisit: "Implement",
        }
        towards_map = {
            schema.Values.StatusExploring: "something new",
            schema.Values.StatusPlanning: "something new",
            schema.Values.StatusImplementing: "something new",
            schema.Values.StatusHabitual: "something regular",
            schema.Values.StatusSatisfied: "something old",
            schema.Values.StatusRevisit: "something past",
        }
        nouns_request = jql_pb2.ListRowsRequest(
            table=schema.Tables.Nouns,
            conditions=[
                jql_pb2.Condition(requires=[
                    jql_pb2.Filter(
                        column=schema.Fields.Parent,
                        in_match=jql_pb2.InMatch(values=sorted(feeds))),
                    jql_pb2.Filter(
                        column=schema.Fields.Status,
                        in_match=jql_pb2.InMatch(values=sorted(action_map))),
                ], ),
            ],
        )
        nouns = self.client.ListRows(nouns_request)
        cmap = {c.name: i for i, c in enumerate(nouns.columns)}
        primary = common.get_primary(nouns)
        children = {}
        for row in nouns.rows:
            pk = row.entries[primary].formatted
            parent = row.entries[cmap[schema.Fields.Parent]].formatted
            local_action_map = dict(action_map)
            if "Feed.Action" in feed_attrs[parent]:
                local_action_map[schema.Values.StatusImplementing] = feed_attrs[parent]["Feed.Action"][0]
                local_action_map[schema.Values.StatusHabitual] = feed_attrs[parent]["Feed.Action"][0]
                local_action_map[schema.Values.StatusSatisfied] = feed_attrs[parent]["Feed.Action"][0]
                local_action_map[schema.Values.StatusRevisit] = feed_attrs[parent]["Feed.Action"][0]
            towards = towards_map[row.entries[cmap[schema.Fields.Status]].formatted]
            action = local_action_map[row.entries[cmap[schema.Fields.Status]].formatted]
            domain = feed_attrs[parent].get("Domain", [''])[0]
            genre = feed_attrs[parent].get("Feed.Genre", [''])[0]
            motivation = feed_attrs[parent].get("Feed.Motivation", [''])[0]
            direct = pk
            if 'yes' in feed_attrs[parent].get("Feed.StripContext", []):
                direct = row.entries[cmap[schema.Fields.Description]].formatted
            children[pk] = {
                "_pk": [f"{action} {pk}"],
                "Action": [action],
                "Direct": [direct],
                "Source": [f"@timedb:{parent}:"],
                "Domain": [domain],
                "Genre": [genre],
                "Motivation": [motivation],
                "Towards": [towards],
            }
        return children

    def GetRow(self, request, context):
        resp = self.ListRows(jql_pb2.ListRowsRequest(), context)
        primary = common.get_primary(resp)
        for row in resp.rows:
            if row.entries[primary].formatted == request.pk:
                return jql_pb2.GetRowResponse(
                    table='vt.practices',
                    columns = resp.columns,
                    row=row,
                )
        raise ValueError(request.pk)


def _type_of(field, foreign):
    if field in foreign:
        return jql_pb2.EntryType.POLYFOREIGN
    # TODO make the object field a foreign field to nouns
    return jql_pb2.EntryType.STRING
