import collections
import json

from timedb import pks, schema
from timedb.virtual_gateway import common

from jql import jql_pb2, jql_pb2_grpc


class ReviewBackend(jql_pb2_grpc.JQLServicer):

    def __init__(self, client):
        super().__init__()
        self.client = client

    def ListRows(self, request, context):
        # TODO support setting this via filter so I can review any cycle
        ancestor_pk = self._query_active_cycle_pk()
        descendants = self._query_descendants(ancestor_pk)
        workstream_progress = self._workstream_progress(descendants)
        rows = {}
        for workstream, progress in workstream_progress.items():
            rows[workstream] = {
                "_pk": [workstream],
                "A Type": ["Workstream"],
                "Description": [workstream],
                "Progress": [str(progress.progress)],
                "Total": [str(progress.total)],
            }
        return common.list_rows('vt.review', rows, request)

    def _workstream_progress(self, tasks):
        progress = self._breakdown_progress(tasks)
        progress.update(self._incremental_progress(tasks))
        return progress

    def _breakdown_progress(self, tasks):
        primary, cmap = common.list_rows_meta(tasks)
        # Explicit breakdowns
        breakdowns = [
            task.entries[cmap[schema.Fields.Direct]].formatted
            for task in tasks.rows if task.entries[cmap[
                schema.Fields.Indirect]].formatted == 'breakdown'
        ]
        projects = [
            task.entries[cmap[schema.Fields.Direct]].formatted
            for task in tasks.rows
            if task.entries[cmap[schema.Fields.Action]].formatted == 'Work'
            and task.entries[cmap[
                schema.Fields.Indirect]].formatted == 'regularity'
        ]
        # TODO if a path-to-match could support multiple values then we could
        # query all projects in parallel instead of in separate requests
        for project in projects:
            resp = self._query_project_workstreams(project)
            primary = common.get_primary(resp)
            for row in resp.rows:
                breakdowns.append(row.entries[primary].formatted)
        all_children = self._query_workstream_children(breakdowns)
        progress = {breakdown: ProgressAmount() for breakdown in breakdowns}
        primary, cmap = common.list_rows_meta(all_children)
        for child in all_children.rows:
            parent = child.entries[cmap[schema.Fields.Parent]].formatted
            status = child.entries[cmap[schema.Fields.Status]].formatted
            progress[parent].total += 1
            if status not in schema.active_statuses():
                progress[parent].progress += 1
        return progress

    def _incremental_progress(self, tasks):
        primary, cmap = common.list_rows_meta(tasks)
        incrementals = [
            task for task in tasks.rows if task.entries[cmap[
                schema.Fields.Indirect]].formatted == 'incrementality'
        ]
        return {}

    def _query_descendants(self, ancestor_pk):
        return self.client.ListRows(
            jql_pb2.ListRowsRequest(
                table=schema.Tables.Tasks,
                conditions=[
                    jql_pb2.Condition(requires=[
                        jql_pb2.Filter(
                            column=schema.Fields.PrimaryGoal,
                            path_to_match=jql_pb2.PathToMatch(
                                value=ancestor_pk),
                        ),
                    ])
                ],
            ))

    def _query_project_workstreams(self, project_pk):
        return self.client.ListRows(
            jql_pb2.ListRowsRequest(
                table=schema.Tables.Nouns,
                conditions=[
                    jql_pb2.Condition(requires=[
                        jql_pb2.Filter(
                            column=schema.Fields.Parent,
                            path_to_match=jql_pb2.PathToMatch(
                                value=project_pk),
                        ),
                        jql_pb2.Filter(
                            column=schema.Fields.Feed,
                            equal_match=jql_pb2.EqualMatch(value='manual'),
                        ),
                    ])
                ],
            ))

    def _query_workstream_children(self, workstreams):
        return self.client.ListRows(
            jql_pb2.ListRowsRequest(
                table=schema.Tables.Nouns,
                conditions=[
                    jql_pb2.Condition(requires=[
                        jql_pb2.Filter(
                            column=schema.Fields.Parent,
                            in_match=jql_pb2.InMatch(values=workstreams),
                        ),
                    ])
                ],
            ))

    def _query_active_cycle_pk(self):
        tasks = self.client.ListRows(
            jql_pb2.ListRowsRequest(
                table=schema.Tables.Tasks,
                conditions=[
                    jql_pb2.Condition(requires=[
                        jql_pb2.Filter(
                            column=schema.Fields.Status,
                            equal_match=jql_pb2.EqualMatch(value=schema.Values.StatusHabitual),
                        ),
                        jql_pb2.Filter(
                            column=schema.Fields.Action,
                            equal_match=jql_pb2.EqualMatch(value='Accomplish'),
                        ),
                        jql_pb2.Filter(
                            column=schema.Fields.Direct,
                            equal_match=jql_pb2.EqualMatch(value='set goals'),
                        ),
                    ])
                ],
            ))
        primary = common.get_primary(tasks)
        return tasks.rows[0].entries[primary].formatted
        


class ProgressAmount(object):

    def __init__(self, progress=0, total=0):
        self.progress = progress
        self.total = total

    def __repr__(self):
        return str(dict(progres=self.progress, total=self.total))
