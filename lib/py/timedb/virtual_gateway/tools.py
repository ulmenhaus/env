import collections
import json

from timedb import pks, schema
from timedb.virtual_gateway import common
from timedb import client_utils

from jql import jql_pb2, jql_pb2_grpc


class ToolsBackend(jql_pb2_grpc.JQLServicer):

    def __init__(self, client):
        super().__init__()
        self.client = client

    def ListRows(self, request, context):
        targets = client_utils.selected_targets(request)
        selected_parent = _extract_selected_parent(request)
        exercises = self._query_exercises(targets, selected_parent)
        return common.list_rows('vt.tools', exercises, request)

    def _query_exercises(self, selected_targets, selected_parent):
        if not selected_targets:
            return {}

        # target2relations: {kit_pk: {tool_pk: relation}}
        target2relations = collections.defaultdict(dict)

        # Collect explicit kit→tool relations from assertions
        rel_request = jql_pb2.ListRowsRequest(
            table=schema.Tables.Assertions,
            conditions=[jql_pb2.Condition(requires=[
                jql_pb2.Filter(
                    column=schema.Fields.Arg0,
                    in_match=jql_pb2.InMatch(
                        values=[f"nouns {t}" for t in selected_targets])),
            ])],
        )
        assertions = self.client.ListRows(rel_request)
        assns_cmap = {c.name: i for i, c in enumerate(assertions.columns)}
        for row in assertions.rows:
            _, kit_pk = row.entries[assns_cmap[schema.Fields.Arg0]].formatted.split(" ", 1)
            tool_raw = row.entries[assns_cmap[schema.Fields.Arg1]].formatted
            if not client_utils.is_foreign(tool_raw):
                continue
            tool_pk = client_utils.strip_foreign_noun(tool_raw)
            relation = row.entries[assns_cmap[schema.Fields.Relation]].formatted
            target2relations[kit_pk][tool_pk] = relation

        # Supplement explicit relations with the implicit relation of being
        # a child of the toolkit
        #
        # TODO taking the union of explicit and implicit relations between
        # nouns is a common enough operation (see e.g. vt.relatives) that it
        # either should have a common library or a virtual table
        child_request = jql_pb2.ListRowsRequest(
            table=schema.Tables.Nouns,
            conditions=[jql_pb2.Condition(requires=[
                jql_pb2.Filter(
                    column=schema.Fields.Parent,
                    in_match=jql_pb2.InMatch(values=list(selected_targets))),
                jql_pb2.Filter(
                    column=schema.Fields.Status,
                    in_match=jql_pb2.InMatch(values=[
                        schema.Values.StatusSatisfied,
                        schema.Values.StatusHabitual,
                    ])),
            ])],
        )
        children = self.client.ListRows(child_request)
        children_cmap = {c.name: i for i, c in enumerate(children.columns)}
        children_primary = client_utils.get_primary(children)
        for child in children.rows:
            tool_pk = child.entries[children_primary].formatted
            kit_pk = child.entries[children_cmap[schema.Fields.Parent]].formatted
            relation = child.entries[children_cmap[schema.Fields.NounRelation]].formatted or "Item"
            target2relations[kit_pk][tool_pk] = relation

        all_tool_pks = list({tp for d in target2relations.values() for tp in d})
        if not all_tool_pks:
            return {}

        fields, _ = client_utils.get_fields_for_items(self.client,
                                                schema.Tables.Nouns,
                                                all_tool_pks)
        taxonomies = set(
            value for d in fields.values()
            for k, vs in d.items() for value in vs if k == "Taxonomy"
        )
        all_subsets = self._query_subsets(taxonomies)
        tool2info = common.get_timing_info(self.client, all_tool_pks)

        attributes = {}
        for kit_pk, tool_relations in target2relations.items():
            for tool, relation in tool_relations.items():
                # TODO we should only include .Item and .Resource in here but we can only
                # do that once all existing relationships have been changed
                excluded_rels = [".KitDomain"]
                if relation in excluded_rels:
                    continue
                tool_attrs = fields.get(tool, {})
                class2actions = {
                    "@{nouns Claimset}": ["Review"],
                    "@{nouns Schema}": ["Review"],
                    "@{nouns Taxonomy}": ["Review"],
                    "@{nouns Technique}": ["Exercise"],
                    "@{nouns Theory}": ["Review"],
                }
                default_actions = ["Ready", "Evaluate"]
                if relation == ".Resource":
                    default_actions = ["Consult"]
                tool_class = tool_attrs.get("Class", [None])[0]
                actions = tool_attrs.get("Feed.Action", class2actions.get(tool_class, default_actions))
                subsets = ['']
                for taxonomy in fields.get(tool, {}).get("Taxonomy", []):
                    subsets += all_subsets[client_utils.strip_foreign_noun(taxonomy)]
                for action in actions:
                    for subset in subsets:
                        # NOTE we take advantage of the fact here that the subset becomes the
                        # indirect parameter of the task to see if the subset is currently active
                        if tool in tool2info and (
                                action, subset) in tool2info[tool].active_actions:
                            continue
                        exercise = f"{action} {tool}"
                        pk = _encode_pk(exercise, selected_parent, kit_pk, subset)
                        attributes[pk] = {
                            "_pk": [pk],
                            "Relation": [relation],
                            "Action": [action],
                            "Class": tool_attrs.get("Class", []),
                            "Class (Secondary)": tool_attrs.get("Class (Secondary)", []),
                            "Days Since": [""],
                            "Days Until": [""],
                            "Direct": [tool],
                            "Parent": [selected_parent],
                            "-> Item": [kit_pk],  # added to ensure the filter still matches
                            "Motivation": ["Preparation"],
                            "Source": [""],
                            "Towards": [""],
                            "Domain": [""],
                            "Skillset": [""],
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
        tax_list = list(map(client_utils.strip_foreign_noun, taxonomies))
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
        primary = client_utils.get_primary(children)
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
        primary = client_utils.get_primary(resp)
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
