import json

from timedb import schema
from timedb.virtual_gateway import common

from jql import jql_pb2, jql_pb2_grpc


class NounsBackend(jql_pb2_grpc.JQLServicer):

    def __init__(self, client):
        super().__init__()
        self.client = client

    def ListRows(self, request, context):
        entries = {}
        project_plans = _query_project_plans(self.client)
        plan2areas = _query_areas_for_plans(self.client, project_plans)
        project_tasks = jql_pb2.ListRowsRequest(
            table=schema.Tables.Nouns,
            conditions=[
                jql_pb2.Condition(requires=[
                    jql_pb2.Filter(
                        column=schema.Fields.Parent,
                        in_match=jql_pb2.InMatch(values=list(project_plans)),
                    ),
                ]),
            ],
        )
        tasks_resp = self.client.ListRows(project_tasks)
        primary, cmap = common.list_rows_meta(tasks_resp)
        keys = [row.entries[primary].formatted for row in tasks_resp.rows]
        fields, _ = common.get_fields_for_items(self.client,
                                                schema.Tables.Nouns, keys)

        for task in tasks_resp.rows:
            pk = task.entries[primary].formatted
            areas = fields[pk]["Area"]
            if not areas or not common.is_foreign(areas[0]):
                continue
            area = common.strip_foreign(areas[0])
            plan = task.entries[cmap[schema.Fields.Parent]].formatted
            plan2areas[plan].add(area)
            entry = {
                "_pk": [pk],
                schema.Fields.Parent: [_area_pk(area, plan)],
                schema.Fields.Feed: "",
            }
            copied = [
                schema.Fields.Identifier,
                schema.Fields.Modifier,
                schema.Fields.Context,
                schema.Fields.Coordinal,
                schema.Fields.Description,
                schema.Fields.Disambiguator,
                schema.Fields.Link,
                schema.Fields.NounRelation,
                schema.Fields.Status,
            ]
            for cp in copied:
                entry[cp] = [task.entries[cmap[cp]].formatted]
            entries[pk] = entry
        for plan, areas in plan2areas.items():
            for area in areas:
                area_pk = _area_pk(area, plan)
                entries[area_pk] = {
                    "_pk": [area_pk],
                    schema.Fields.Identifier: [area_pk],
                    schema.Fields.Modifier: [""],
                    schema.Fields.Context: [""],
                    schema.Fields.Coordinal: [""],
                    schema.Fields.Description: [area],
                    schema.Fields.Disambiguator: [plan],
                    schema.Fields.Feed: ["manual"],
                    schema.Fields.Link: [""],
                    schema.Fields.Parent: [""],
                    schema.Fields.NounRelation: [""],
                    schema.Fields.Status: ["Habitual"],
                }
        return common.list_rows('vt.project_initiative_nouns', entries,
                                request)

    def WriteRow(self, request, context):
        if request.update_only:
            # We ignore update requests which the feed tool uses
            # to set the coordinals on nouns
            return jql_pb2.WriteRowResponse()
        parent_pk = request.fields[schema.Fields.Parent]
        parent_resp = common.get_row(
            self.ListRows(jql_pb2.ListRowsRequest(), context), parent_pk)
        primary, cmap = common.list_rows_meta(parent_resp)
        area = parent_resp.row.entries[cmap[
            schema.Fields.Description]].formatted
        project = parent_resp.row.entries[cmap[
            schema.Fields.Disambiguator]].formatted
        request.fields[schema.Fields.Parent] = project
        request.table = schema.Tables.Nouns
        self.client.WriteRow(request)
        self.client.WriteRow(
            jql_pb2.WriteRowRequest(
                table=schema.Tables.Assertions,
                pk=f"({request.pk}, Area)",
                fields={
                    schema.Fields.Relation: ".Area",
                    schema.Fields.Arg0: f"nouns {request.pk}",
                    schema.Fields.Arg1: f"@{{nouns {area}}}",
                    schema.Fields.Order: "0",
                },
            ))
        return jql_pb2.WriteRowResponse()

    def GetRow(self, request, context):
        return common.get_row(
            self.ListRows(jql_pb2.ListRowsRequest(), context), request.pk)


class AssertionsBackend(jql_pb2_grpc.JQLServicer):

    def __init__(self, client):
        super().__init__()
        self.client = client

    def ListRows(self, request, context):
        entries = {}
        project_plans = _query_project_plans(self.client)
        plan2areas = _query_areas_for_plans(self.client, project_plans)
        for project_plan, areas in plan2areas.items():
            description = project_plans[project_plan]
            for area in areas:
                entries[f"{project_plan}.{area}"] = {
                    "_pk": [f"{project_plan}.{area}"],
                    schema.Fields.Relation: [".Domain"],
                    schema.Fields.Arg0:
                    [f"nouns {_area_pk(area, project_plan)}"],
                    schema.Fields.Arg1: [f"@{{nouns {description}}}"],
                    schema.Fields.Order: ["0"],
                }
        return common.list_rows('vt.project_initiative_assertions',
                                entries,
                                request,
                                allow_foreign=False)


def _query_project_plans(client):
    active_projects_req = jql_pb2.ListRowsRequest(
        table=schema.Tables.Nouns,
        conditions=[
            jql_pb2.Condition(requires=[
                jql_pb2.Filter(
                    column=schema.Fields.Status,
                    in_match=jql_pb2.InMatch(values=schema.active_statuses()),
                ),
                jql_pb2.Filter(
                    column=schema.Fields.Modifier,
                    equal_match=jql_pb2.EqualMatch(
                        value=schema.Values.ModifierPlanFor),
                ),
            ]),
        ],
    )

    active_resp = client.ListRows(active_projects_req)
    primary, cmap = common.list_rows_meta(active_resp)
    return {
        row.entries[primary].formatted:
        row.entries[cmap[schema.Fields.Description]].formatted
        for row in active_resp.rows
    }


def _query_areas_for_plans(client, plans):
    taxonomies = set()
    plan2tax = {plan: [] for plan in plans}
    fields, _ = common.get_fields_for_items(client, schema.Tables.Nouns, list(plans))
    for plan, attr_set in fields.items():
        for taxonomy in attr_set['Taxonomy']:
            if common.is_foreign(taxonomy):
                taxonomies.add(common.strip_foreign(taxonomy))
                plan2tax[plan].append(common.strip_foreign(taxonomy))
    children_req = jql_pb2.ListRowsRequest(
        table=schema.Tables.Nouns,
        conditions=[
            jql_pb2.Condition(requires=[
                jql_pb2.Filter(
                    column=schema.Fields.Parent,
                    in_match=jql_pb2.InMatch(values=sorted(taxonomies)),
                ),
            ]),
        ],
    )
    children_resp = client.ListRows(children_req)
    primary, cmap = common.list_rows_meta(children_resp)
    tax2areas = {tax: [] for tax in taxonomies}
    for row in children_resp.rows:
        tax2areas[row.entries[cmap[schema.Fields.Parent]].formatted].append(
            row.entries[primary].formatted)
    plan2areas = {plan: set() for plan in plans}
    for plan, taxonomies in plan2tax.items():
        for tax in taxonomies:
            for area in tax2areas[tax]:
                plan2areas[plan].add(area)
    return plan2areas


def _area_pk(area, plan):
    return f"{area} ({plan})"
