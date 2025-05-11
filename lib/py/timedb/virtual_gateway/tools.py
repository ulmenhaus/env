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
        return common.list_rows('vt.tools', exercises, request)

    def _query_exercises(self, selected_target, selected_parent):
        if not selected_target:
            return {}
        requires = jql_pb2.Filter(
            column=schema.Fields.Arg0,
            equal_match=jql_pb2.EqualMatch(value=f"nouns {selected_target}"))
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
            target_pk = common.strip_foreign_noun(target)
            relation = row.entries[cmap[schema.Fields.Relation]].formatted
            target2relation[target_pk] = relation

        # Supplement explicit relations with the implicit relation of being
        # a child of the toolkit
        #
        # TODO taking the union of explicit and implicit relations between
        # nouns is a common enough operation (see e.g. vt.relatives) that it
        # either should have a common library or a virtual table
        requires = [
            jql_pb2.Filter(
                column=schema.Fields.Parent,
                equal_match=jql_pb2.EqualMatch(value=selected_target)),
            jql_pb2.Filter(
                column=schema.Fields.Status,
                in_match=jql_pb2.InMatch(
                    values=[schema.Values.StatusSatisfied, schema.Values.StatusHabitual])),
        ]
        child_request = jql_pb2.ListRowsRequest(
            table=schema.Tables.Nouns,
            conditions=[
                jql_pb2.Condition(requires=requires),
            ])
        children = self.client.ListRows(child_request)
        cmap = {c.name: i for i, c in enumerate(children.columns)}
        primary = common.get_primary(children)
        for child in children.rows:
            target2relation[child.entries[primary].formatted] = child.entries[
                cmap[schema.Fields.NounRelation]].formatted or "Item"

        fields, _ = common.get_fields_for_items(self.client,
                                                schema.Tables.Nouns,
                                                list(target2relation.keys()))
        taxonomies = set([
            value for d in fields.values() for k, v in d.items() for value in v
            if k == "Taxonomy"
        ])
        all_subsets = self._query_subsets(taxonomies)
        tool2info = common.get_timing_info(self.client, target2relation.keys())
        for tool, relation in target2relation.items():
            excluded_rels = [".KitDomain"]
            if all([rel in excluded_rels for rel in relation]):
                continue
            default_actions = ["Exercise", "Ready", "Evaluate", "Review"]
            if relation == [".Resource"]:
                default_actions = ["Consult"]
            actions = fields.get(tool, {}).get("Feed.Action",
                                                    []) or default_actions
            subsets = ['']
            for taxonomy in fields.get(tool, {}).get("Taxonomy", []):
                subsets += all_subsets[common.strip_foreign_noun(taxonomy)]
            for action in actions:
                for subset in subsets:
                    # NOTE we take advantage of the fact here that the subset becomes the
                    # indirect parameter of the task to see if the subset is currently active
                    if tool in tool2info and (action, subset) in tool2info[tool].active_actions:
                        continue
                    exercise = f"{action} {tool}"
                    pk = _encode_pk(exercise, selected_parent, selected_target,
                                    subset)
                    attributes[pk] = {
                        "_pk": [pk],
                        "Relation": [relation],
                        "Action": [action],
                        "Direct": [tool],
                        "Parent": [selected_parent],
                        "-> Item":
                        [selected_target
                         ],  # added to ensure the filter still matches
                        "Motivation": ["Preparation"],
                        "Source": [""],
                        "Towards": [""],
                        "Domain": [""],
                        "Subset": [subset],
                    }
                    if tool in tool2info:
                        # NOTE for now days since/until is shared by all subsets, but
                        # we could make it more granular in the future. The expectation is
                        # that picking a broad exercise by its timing is good enough. If subsets
                        # are important enough, they can show up in vt.habituals for periodic review.
                        info = tool2info[tool]
                        attributes[pk]['Days Since'] = [info.days_since]
                        attributes[pk]['Days Until'] = [info.days_until]
        return attributes

    def _query_subsets(self, taxonomies):
        tax_list = list(map(common.strip_foreign_noun, taxonomies))
        request = jql_pb2.ListRowsRequest(
            table=schema.Tables.Nouns,
            conditions=[
                jql_pb2.Condition(requires=[
                    jql_pb2.Filter(column=schema.Fields.Parent,
                                   in_match=jql_pb2.InMatch(values=tax_list)),
                    jql_pb2.Filter(column=schema.Fields.Status,
                                   equal_match=jql_pb2.EqualMatch(
                                       value=schema.Values.StatusHabitual)),
                ]),
            ],
        )
        children = self.client.ListRows(request)
        cmap = {c.name: i for i, c in enumerate(children.columns)}
        primary = common.get_primary(children)
        subsets = collections.defaultdict(list)
        for row in children.rows:
            subsets[row.entries[cmap[schema.Fields.Parent]].formatted].append(
                row.entries[primary].formatted)
        return subsets

    def GetRow(self, request, context):
        _, parent, target, _ = _decode_pk(request.pk)
        resp = self.ListRows(
            jql_pb2.ListRowsRequest(conditions=[
                jql_pb2.Condition(requires=[
                    jql_pb2.Filter(
                        column='Parent',
                        equal_match=jql_pb2.EqualMatch(value=parent)),
                    jql_pb2.Filter(
                        column='-> Item',
                        equal_match=jql_pb2.EqualMatch(value=target)),
                ], ),
            ], ), context)
        primary = common.get_primary(resp)
        for row in resp.rows:
            if row.entries[primary].formatted == request.pk:
                return jql_pb2.GetRowResponse(
                    table='vt.tools',
                    columns=resp.columns,
                    row=row,
                )
        raise ValueError(request.pk)


def _extract_selected_parent(request):
    for condition in request.conditions:
        for f in condition.requires:
            match_type = f.WhichOneof('match')
            if match_type == "equal_match" and f.column == 'Parent':
                return f.equal_match.value
    return ""


def _encode_pk(exercise, parent, target, subset):
    return "\t".join([exercise, parent, target, subset])


def _decode_pk(pk):
    exercise, parent, target, subset = pk.split("\t")
    return exercise, parent, target, subset
