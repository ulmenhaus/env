import collections
import json

from timedb import schema
from timedb.virtual_gateway import common

from jql import jql_pb2, jql_pb2_grpc


class AttributesBackend(jql_pb2_grpc.JQLServicer):

    def __init__(self, client):
        super().__init__()
        self.client = client

    def ListRows(self, request, context):
        selected_target = common.selected_target(request)
        if not selected_target:
            return common.possible_targets(self.client, request, 'vt.attributes')

        attributes = self._query_attributes(selected_target)
        foreign_fields = _foreign_fields(attributes.values())
        initial = _convert_foreign_fields(attributes.values(), foreign_fields)
        grouped, groupings = common.apply_grouping(initial, request)
        max_lens = common.gather_max_lens(grouped, [])
        final, all_count = common.apply_request_parameters(grouped, request)
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
                "-> Item": [selected_target],
            }
        return attributes

    def WriteRow(self, request, context):
        raise Exception("not implemented")


def is_verb(attribute):
    return attribute.endswith("es")


def _type_of(field, foreign):
    if field == "Display Name":
        return jql_pb2.EntryType.POLYFOREIGN
    if field in foreign:
        return jql_pb2.EntryType.POLYFOREIGN
    return jql_pb2.EntryType.STRING


def _is_foreign(entry):
    return len(entry) > len("@timedb:") and entry.startswith(
        "@timedb:") and entry.endswith(":") and ":" not in _strip_foreign(
            entry)


def _strip_foreign(entry):
    return entry[len("@timedb:"):-1]


def _foreign_fields(rows):
    all_fields = set()
    not_foreign = set()
    for row in rows:
        for k, v in row.items():
            all_fields.add(k)
            for item in v:
                if not _is_foreign(item):
                    not_foreign.add(k)
    return all_fields - not_foreign


def _convert_foreign_fields(before, foreign_fields):
    after = []
    for row in before:
        new_row = collections.defaultdict(list)
        for k, v in row.items():
            if k in foreign_fields:
                # For now we only allow referencing nouns from assertions, but we may support other tables in the future
                new_row[k] = [f"nouns {_strip_foreign(item)}" for item in v]
            else:
                new_row[k] = v
        after.append(new_row)
    return after
