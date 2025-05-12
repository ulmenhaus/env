import collections
import json

from timedb import schema, pks
from timedb.virtual_gateway import common

from jql import jql_pb2, jql_pb2_grpc


class RelativesBackend(jql_pb2_grpc.JQLServicer):

    def __init__(self, client):
        super().__init__()
        self.client = client
        # A user can use the new entry UI to add columns to the current visualization
        self.extra_columns = set()
        self.extra_columns_target = None

    def ListRows(self, request, context):
        selected_target = common.selected_target(request)
        if not selected_target:
            return common.possible_targets(self.client, request,
                                           'vt.relatives')

        if self.extra_columns_target != selected_target:
            # Reset the extra columns because we're looking at a new target
            self.extra_columns = set()
            self.extra_columns_target = selected_target
        selected_table, selected_item = selected_target.split(" ", 1)
        relatives = {}
        if selected_table == schema.Tables.Nouns:
            relatives.update(self._query_explicit_relatives(selected_item))
            relatives.update(
                self._query_explicit_task_relatives(selected_item))
            relatives.update(
                self._query_implied_relatives(selected_item,
                                              schema.Tables.Nouns,
                                              schema.Fields.Parent, "Child"))
            relatives.update(
                self._query_implied_relatives(selected_item,
                                              schema.Tables.Nouns,
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
                self._query_implied_relatives(selected_item,
                                              schema.Tables.Nouns,
                                              schema.Fields.Description,
                                              "w/ Base Concept"))
            relatives.update(
                self._query_implied_relatives(selected_item,
                                              schema.Tables.Nouns,
                                              schema.Fields.Identifier,
                                              "w/ Identity"))
            if selected_item == schema.SpecialClassesForRelatives.FeedClass:
                # Nouns with a non-empty Feed field are implicitly
                # instances of the Feed class
                relatives.update(
                    self._query_implied_relatives(selected_item,
                                                  schema.Tables.Nouns,
                                                  schema.Fields.Feed,
                                                  "w/ Class",
                                                  negated=True,
                                                  value=""))

        elif selected_table == schema.Tables.Tasks:
            relatives.update(
                self._query_implied_relatives(selected_item,
                                              schema.Tables.Tasks,
                                              schema.Fields.PrimaryGoal,
                                              "Child"))
            relatives.update(
                self._query_implied_relatives(selected_item,
                                              schema.Tables.Tasks,
                                              schema.Fields.UDescription,
                                              "w/ Identity"))
        for fields in relatives.values():
            for extra_col in self.extra_columns:
                if extra_col not in fields:
                    fields[extra_col] = [""]
        return common.list_rows('vt.relatives', relatives, request)

    def _query_explicit_relatives(self, selected_item):
        arg1 = f"@{{nouns {selected_item}}}"
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
            relative["Display Name"] = [f"@{{{pk}}}"]
            relative["-> Item"] = [f"{schema.Tables.Nouns} {selected_item}"]
            exact_matches = [k for k, v in relative.items() if arg1 in v]
            if exact_matches:
                rel = exact_matches[0]
                relative["Relation"] = [f"{rel} this"
                                        ] if is_verb(rel) else [f"w/ {rel}"]
            else:
                relative["Relation"] = ["w/ Mention"]
            # TODO two edge cases for the relation
            # 1. If it's a verb like "Defines" we want it to be "which define this {class}"
            # 2. If there isn't an exact match we'll say "which reference this {class}"
        return relatives

    def _query_explicit_task_relatives(self, selected_item):
        actions = self.client.ListRows(
            jql_pb2.ListRowsRequest(table=schema.Tables.Actions))
        action_primary, action_cmap = common.list_rows_meta(actions)
        actions_by_primary = {
            action.entries[action_primary].formatted: action
            for action in actions.rows
        }
        tasks = {}
        # TODO once multiple conditions are allowed we can make this request in
        # a single query
        for column in [schema.Fields.Direct, schema.Fields.Indirect]:
            relatives_request = jql_pb2.ListRowsRequest(
                table=schema.Tables.Tasks,
                conditions=[
                    jql_pb2.Condition(requires=[
                        jql_pb2.Filter(
                            column=column,
                            equal_match=jql_pb2.EqualMatch(value=selected_item,
                                                           ),
                        ),
                    ]),
                ],
            )
            response = self.client.ListRows(relatives_request)
            primary, cmap = common.list_rows_meta(response)
            for row in response.rows:
                tasks[row.entries[primary].formatted] = row
        relatives, assn_pks = common.get_fields_for_items(
            self.client, "tasks", list(tasks))
        for pk, relative in relatives.items():
            relation = "w/ Unknown"
            task = tasks[pk]
            action_primary = task.entries[cmap[schema.Fields.Action]].formatted
            if action_primary in actions_by_primary:
                action = actions_by_primary[action_primary]
                direct = task.entries[cmap[schema.Fields.Direct]].formatted
                if direct == selected_item:
                    ps = action.entries[action_cmap[
                        schema.Fields.Direct]].formatted
                    relation = f"w/ {schema.relation_from_parameter_schema(ps)}"
                indirect = task.entries[cmap[schema.Fields.Indirect]].formatted
                if indirect == selected_item:
                    ps = action.entries[action_cmap[
                        schema.Fields.Indirect]].formatted
                    relation = f"w/ {schema.relation_from_parameter_schema(ps)}"
            relative["_pk"] = [common.encode_pk(f"tasks {pk}", assn_pks[pk])]
            relative["Display Name"] = [f"@{{tasks {pk}}}"]
            relative["-> Item"] = [f"{schema.Tables.Nouns} {selected_item}"]
            relative["Relation"] = [relation]
        return relatives

    def _query_implied_relatives(
        self,
        selected_item,
        table,
        field,
        relation,
        path_to_match=False,
        negated=False,
        value=None,
    ):
        to_match = value if value is not None else selected_item
        requires = jql_pb2.Filter(
            column=field,
            negated=negated,
            equal_match=jql_pb2.EqualMatch(value=to_match))
        if path_to_match:
            requires = jql_pb2.Filter(
                column=field,
                negated=negated,
                path_to_match=jql_pb2.PathToMatch(value=to_match))
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
            relative["Display Name"] = [f"@{{{pk}}}"]
            relative["-> Item"] = [f"{table} {selected_item}"]
            relative["Relation"] = [relation]
        return relatives

    def WriteRow(self, request, context):
        if not common.is_encoded_pk(request.pk):
            # This pk was provided by the user to add a column to the UI
            self.extra_columns.add(request.pk)
            return jql_pb2.WriteRowResponse()
        pk, pk_map = common.decode_pk(request.pk)
        for field, value in request.fields.items():
            new_entries = _from_bulleted_list(value)
            for assn_pk, _attr_value in pk_map.get(field, []):
                self.client.DeleteRow(
                    jql_pb2.DeleteRowRequest(
                        table=schema.Tables.Assertions,
                        pk=assn_pk,
                    ))
            for i, new_entry in enumerate(new_entries):
                fields = {
                    schema.Fields.Relation: f".{field}",
                    schema.Fields.Arg0: pk,
                    schema.Fields.Arg1: new_entry,
                    schema.Fields.Order: str(i),
                }
                self.client.WriteRow(
                    jql_pb2.WriteRowRequest(
                        table=schema.Tables.Assertions,
                        pk=pks.pk_for_assertion(fields),
                        fields=fields,
                        insert_only=True,
                    ))
        return jql_pb2.WriteRowResponse()

    def DeleteRow(self, request, context):
        entry_pk, assn_pks = common.decode_pk(request.pk)
        for assns in assn_pks.values():
            for assn_pk, _ in assns:
                self.client.DeleteRow(
                    jql_pb2.DeleteRowRequest(
                        table=schema.Tables.Assertions,
                        pk=assn_pk,
                    ))
        return jql_pb2.DeleteRowResponse()

    def GetRow(self, request, context):
        pk, pk_map = common.decode_pk(request.pk)
        mapping = {
            "_pk": [request.pk],
        }
        for attr_name, attr_pairs in pk_map.items():
            if len(attr_pairs) == 0:
                mapping[attr_name] = [""]
            elif len(attr_pairs) == 1:
                mapping[attr_name] = [attr_pairs[0][1]]
            else:
                # Convert multiple attributes to a bulleted list so that
                # they can be edited as one text blob
                mapping[attr_name] = [
                    _to_bulleted_list(value for _pk, value in attr_pairs)
                ]
        return common.return_row('vt.relatives', mapping)


def _to_bulleted_list(entries):
    return "\n".join(f"* {entry}" for entry in entries)


def _from_bulleted_list(blob):
    if not blob:
        return []
    if not blob.startswith("* "):
        return [blob]
    return blob[2:].split("\n* ")


def is_verb(attribute):
    return attribute.endswith("es")
