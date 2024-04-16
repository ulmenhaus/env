import collections
import json

from timedb import schema
from timedb.virtual_gateway import common

from jql import jql_pb2, jql_pb2_grpc


class RelativesBackend(jql_pb2_grpc.JQLServicer):

    def __init__(self, client):
        super().__init__()
        self.client = client

    def ListRows(self, request, context):
        selected_target = common.selected_target(request)
        if not selected_target:
            return common.possible_targets(self.client, request, 'vt.relatives')

        selected_table, selected_item = selected_target.split(" ", 1)
        relatives = {}
        if selected_table == schema.Tables.Nouns:
            relatives.update(self._query_explicit_relatives(selected_item))
            relatives.update(
                self._query_implied_relatives(selected_item, schema.Tables.Nouns,
                                              schema.Fields.Parent, "Child"))
            relatives.update(
                self._query_implied_relatives(selected_item, schema.Tables.Nouns,
                                              schema.Fields.Modifier,
                                              "w/ Modifier"))
            # We query all descendants first so that more immediate relatives will override
            # with more specific relations
            relatives.update(
                self._query_implied_relatives(selected_item,
                                              schema.Tables.Nouns,
                                              schema.Fields.Description,
                                              "w/ Ancestor Concept",
                                              path_to_match=True))
            relatives.update(
                self._query_implied_relatives(selected_item, schema.Tables.Nouns,
                                              schema.Fields.Description,
                                              "w/ Base Concept"))
            relatives.update(
                self._query_implied_relatives(selected_item, schema.Tables.Nouns,
                                              schema.Fields.Identifier,
                                              "w/ Identity"))
        elif selected_table == schema.Tables.Tasks:
            relatives.update(
                self._query_implied_relatives(selected_item, schema.Tables.Tasks,
                                              schema.Fields.PrimaryGoal, "Child"))
            relatives.update(
                self._query_implied_relatives(selected_item, schema.Tables.Tasks,
                                              schema.Fields.UDescription,
                                              "w/ Identity"))
        # TODO Captured implied relations
        # 1. From arguments (direct/indirect of tasks, modified for nouns)
        # 2. Items which use a particular schema (as referenced by parent)
        first_fields = ["Display Name", "Class", "Relation"]
        grouped, groupings = common.apply_grouping(relatives.values(), request)
        max_lens = common.gather_max_lens(grouped, first_fields)
        filtered, all_count = common.apply_request_parameters(grouped, request)
        foreign_fields = common.foreign_fields(filtered)
        final = common.convert_foreign_fields(filtered, foreign_fields)
        shared_fields = sorted(set().union(*(final)) - set(first_fields) -
                               {"_pk", "-> Item"})
        fields = first_fields + shared_fields + ["_pk"]
        return jql_pb2.ListRowsResponse(
            table='vt.relatives',
            columns=[
                jql_pb2.Column(name=field,
                               type=_type_of(field, foreign_fields),
                               max_length=max_lens.get(field, len(field)),
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
            all=len(relatives),
            groupings=groupings,
        )

    def _query_explicit_relatives(self, selected_item):
        arg1 = f"@timedb:{selected_item}:"
        relatives_request = jql_pb2.ListRowsRequest(
            table=schema.Tables.Assertions,
            conditions=[
                jql_pb2.Condition(requires=[
                    jql_pb2.Filter(
                        column=schema.Fields.Arg1,
                        contains_match=jql_pb2.ContainsMatch(value=arg1, ),
                    ),
                ]),
            ],
        )
        relatives_response = self.client.ListRows(relatives_request)
        assn_cmap = {
            c.name: i
            for i, c in enumerate(relatives_response.columns)
        }
        arg0s = {
            row.entries[assn_cmap[schema.Fields.Arg0]].formatted
            for row in relatives_response.rows
        }
        relatives, assn_pks = common.get_fields_for_items(
            self.client, "", arg0s)
        for pk, relative in relatives.items():
            relative["_pk"] = [common.encode_pk(pk, assn_pks[pk])]
            relative["Display Name"] = [pk]
            relative["-> Item"] = [f"{schema.Tables.Nouns} {selected_item}"]
            exact_matches = [k for k, v in relative.items() if arg1 in v]
            if exact_matches:
                rel = exact_matches[0]
                relative["Relation"] = [f"{rel} this"] if is_verb(rel) else [f"w/ {rel}"]
            # TODO two edge cases for the relation
            # 1. If it's a verb like "Defines" we want it to be "which define this {class}"
            # 2. If there isn't an exact match we'll say "which reference this {class}"
        return relatives

    def _query_implied_relatives(self,
                                 selected_item,
                                 table,
                                 field,
                                 relation,
                                 path_to_match=False):
        requires = jql_pb2.Filter(
            column=field,
            equal_match=jql_pb2.EqualMatch(value=selected_item))
        if path_to_match:
            requires = jql_pb2.Filter(
                column=field,
                path_to_match=jql_pb2.PathToMatch(value=selected_item))
        rel_request = jql_pb2.ListRowsRequest(
            table=table,
            conditions=[
                jql_pb2.Condition(requires=[requires]),
            ],
        )
        rel_response = self.client.ListRows(rel_request)
        primary = common.get_primary(rel_response)
        arg0s = {
            f"{table} {row.entries[primary].formatted}"
            for row in rel_response.rows
        }
        relatives, assn_pks = common.get_fields_for_items(
            self.client, "", arg0s)
        for pk, relative in relatives.items():
            relative["_pk"] = [common.encode_pk(pk, assn_pks[pk])]
            relative["Display Name"] = [pk]
            relative["-> Item"] = [f"{table} {selected_item}"]
            relative["Relation"] = [relation]
        return relatives

    def WriteRow(self, request, context):
        pk, pk_map = common.decode_pk(request.pk)
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
            else:
                request = jql_pb2.WriteRowRequest(
                    table=schema.Tables.Assertions,
                    pk=str((f".{field}", pk, "0000")),
                    fields={
                        schema.Fields.Relation: f".{field}",
                        schema.Fields.Arg0: pk,
                        schema.Fields.Arg1: value,
                    },
                    insert_only=True,
                )
                self.client.WriteRow(request)
        return jql_pb2.WriteRowResponse()


def is_verb(attribute):
    return attribute.endswith("es")

def _type_of(field, foreign):
    if field == "Display Name":
        return jql_pb2.EntryType.POLYFOREIGN
    if field in foreign:
        return jql_pb2.EntryType.POLYFOREIGN
    return jql_pb2.EntryType.STRING
