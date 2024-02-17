import json

from timedb import schema
from timedb.virtual_gateway import common

from jql import jql_pb2, jql_pb2_grpc


class RelativesBackend(jql_pb2_grpc.JQLServicer):

    def __init__(self, client):
        super().__init__()
        self.client = client

    def ListRows(self, request, context):
        selected_target = _selected_target(request)
        if not selected_target:
            return self._possible_targets(request)

        relatives = self._query_explicit_relatives(selected_target)
        relatives.update(
            self._query_implied_relatives(selected_target, schema.Tables.Nouns,
                                          schema.Fields.Parent, "Child"))
        relatives.update(
            self._query_implied_relatives(selected_target, schema.Tables.Nouns,
                                          schema.Fields.Modifier,
                                          "w/ Modifier"))
        # We query all descendants first so that more immediate relatives will override
        # with more specific relations
        relatives.update(
            self._query_implied_relatives(selected_target,
                                          schema.Tables.Nouns,
                                          schema.Fields.Description,
                                          "w/ Ancestor Concept",
                                          path_to_match=True))
        relatives.update(
            self._query_implied_relatives(selected_target, schema.Tables.Nouns,
                                          schema.Fields.Description,
                                          "w/ Base Concept"))
        relatives.update(
            self._query_implied_relatives(selected_target, schema.Tables.Nouns,
                                          schema.Fields.Identifier,
                                          "w/ Identity"))
        # TODO Captured implied relations
        # 1. Children
        # 2. From arguments (direct/indirect of tasks, modified for nouns)
        # 3. Inherited relationships (e.g. object whose class is a sub-class of this one or a descendant of the modifier noun tree "core concept")
        # 4. Items which use a particular schema (as referenced by parent)
        first_fields = ["Display Name", "Class", "Relation"]
        max_lens = common.gather_max_lens(relatives.values(), first_fields)
        final, all_count = common.apply_request_parameters(
            relatives.values(), request)
        shared_fields = sorted(set().union(*(final)) - set(first_fields) -
                               {"_pk", "-> Item"})
        fields = first_fields + shared_fields + ["_pk", "_link"]
        return jql_pb2.ListRowsResponse(
            table='vt.relatives',
            columns=[
                jql_pb2.Column(name=field,
                               type=_type_of(field),
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
            all=len(relatives),
        )

    def _possible_targets(self, request):
        nouns_request = jql_pb2.ListRowsRequest(table=schema.Tables.Nouns, )
        nouns_response = self.client.ListRows(nouns_request)
        primary, = [
            i for i, c in enumerate(nouns_response.columns) if c.primary
        ]
        nouns_cmap = {c.name: i for i, c in enumerate(nouns_response.columns)}
        noun_pks = [
            f"{row.entries[primary].formatted}" for row in nouns_response.rows
        ]
        entries = [{"_pk": [pk], "-> Item": [pk]} for pk in noun_pks]
        final, all_count = common.apply_request_parameters(entries, request)
        return jql_pb2.ListRowsResponse(
            table='vt.relatives',
            columns=[
                jql_pb2.Column(name="-> Item", max_length=30, primary=True)
            ],
            rows=[
                jql_pb2.Row(
                    entries=[jql_pb2.Entry(formatted=noun_pk["_pk"][0])])
                for noun_pk in final
            ],
            total=all_count,
            all=len(entries),
        )

    def _query_explicit_relatives(self, selected_target):
        arg1 = f"@timedb:{selected_target}:"
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
            relative["_link"] = [pk]
            relative["Display Name"] = [pk.split(" ", 1)[-1]]
            relative["-> Item"] = [selected_target]
            exact_matches = [k for k, v in relative.items() if arg1 in v]
            if exact_matches:
                rel = exact_matches[0]
                relative["Relation"] = [f"{rel} this"] if is_verb(rel) else [f"w/ {rel}"]
            # TODO two edge cases for the relation
            # 1. If it's a verb like "Defines" we want it to be "which define this {class}"
            # 2. If there isn't an exact match we'll say "which reference this {class}"
        return relatives

    def _query_implied_relatives(self,
                                 selected_target,
                                 table,
                                 field,
                                 relation,
                                 path_to_match=False):
        requires = jql_pb2.Filter(
            column=field,
            equal_match=jql_pb2.EqualMatch(value=selected_target))
        if path_to_match:
            requires = jql_pb2.Filter(
                column=field,
                path_to_match=jql_pb2.PathToMatch(value=selected_target))
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
            relative["_link"] = [pk]
            relative["Display Name"] = [pk.split(" ", 1)[-1]]
            relative["-> Item"] = [selected_target]
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


def _selected_target(request):
    for condition in request.conditions:
        for f in condition.requires:
            match_type = f.WhichOneof('match')
            if match_type == "equal_match" and f.column == '-> Item':
                return f.equal_match.value

def is_verb(attribute):
    return attribute.endswith("es")

def _type_of(field):
    if field == "_link":
        return jql_pb2.EntryType.POLYFOREIGN
    return jql_pb2.EntryType.STRING
