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
        arg1 = f"@timedb:{selected_target}:"

        # Query for any related items
        relatives_request = jql_pb2.ListRowsRequest(
            table=schema.Tables.Assertions,
            conditions=[
                jql_pb2.Condition(requires=[
                    jql_pb2.Filter(
                        column=schema.Fields.Arg1,
                        contains_match=jql_pb2.ContainsMatch(
                            value=arg1,
                        ),
                    ),
                ]),
            ],
        )
        relatives_response = self.client.ListRows(relatives_request)
        assn_cmap = {c.name: i for i, c in enumerate(relatives_response.columns)}
        arg0s = {row.entries[assn_cmap[schema.Fields.Arg0]].formatted for row in relatives_response.rows}
        relatives, _ = common.get_fields_for_items(self.client, "", arg0s)
        for pk, relative in relatives.items():
            relative["_pk"] = [pk]
            relative["-> Item"] = [selected_target]
            exact_matches = [k for k, v in relative.items() if arg1 in v]
            if exact_matches:
                relative["Relation"] = [f"with this {exact_matches[0]}"]
            # TODO two edge cases for the relation
            # 1. If it's a verb like "Defines" we want it to be "which define this {class}"
            # 2. If there isn't an exact match we'll say "which reference this {class}"

        # TODO Captured implied relations
        # 1. Children
        # 2. From arguments (direct/indirect of tasks, modified for nouns)
        # 3. Inherited relationships (e.g. object whose class is a sub-class of this one)
        max_lens = common.gather_max_lens(relatives.values())
        final, all_count = common.apply_request_parameters(relatives.values(), request)
        first_fields = ["Class", "Relation"]
        shared_fields = sorted(set().union(*(final)) - set(first_fields) - {"_pk", "-> Item"})
        fields = first_fields + shared_fields + ["_pk"]
        return jql_pb2.ListRowsResponse(
            table='vt.relatives',
            columns=[
                jql_pb2.Column(name=field,
                               max_length=max_lens[field],
                               primary=field == '_pk') for field in fields
            ],
            rows=[
                jql_pb2.Row(entries=[
                    jql_pb2.Entry(formatted=common.present_attrs(relative[field])) for field in fields
                ]) for relative in final
            ],
            total=all_count,
            all=len(relatives),
        )
    def _possible_targets(self, request):
        nouns_request = jql_pb2.ListRowsRequest(
            table=schema.Tables.Nouns,
        )
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
                jql_pb2.Column(name="-> Item",
                               max_length=30,
                               primary=True)
            ],
            rows=[
                jql_pb2.Row(entries=[
                    jql_pb2.Entry(formatted=noun_pk["_pk"][0])
                ]) for noun_pk in final
            ],
            total=all_count,
            all=len(entries),
        )

def _selected_target(request):
    for condition in request.conditions:
        for f in condition.requires:
            match_type = f.WhichOneof('match')
            if match_type == "equal_match" and f.column == '-> Item':
                return f.equal_match.value

