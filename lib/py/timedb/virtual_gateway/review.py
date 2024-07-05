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
        rows = {}
        rows.update(self._workstream_entries(descendants))
        return common.list_rows('vt.review', rows, request)

    def _workstream_entries(self, tasks):
        progresses = self._breakdown_progress(tasks)
        progresses.update(self._incremental_progress(tasks))
        entries = {}
        pk2task = {}
        primary, cmap = common.list_rows_meta(tasks)
        for task in tasks.rows:
            pk2task[task.entries[primary].formatted] = task
        for workstream, progress in progresses.items():
            if workstream in pk2task:
                entry_pk = f"@{{tasks {workstream}}}"
                task = pk2task[workstream]
                action = task.entries[cmap[schema.Fields.Action]].formatted
                direct = task.entries[cmap[schema.Fields.Direct]].formatted
                entry_description = f"{action} {direct}"
            else:
                entry_pk = f"@{{nouns {workstream}}}"
                entry_description = workstream
            entries[workstream] = {
                "_pk": [entry_pk],
                "A Type": ["Workstream"],
                # TODO technically I need to include any prepositions from the action
                # so should use the pk library, but this is good enough for most purposes
                "Description": [entry_description],
                "Progress": [str(progress.progress)],
                "Total": [str(progress.total)],
                "Z %%": [
                    f"{(str(progress.percentage()) + '%%').ljust(3)} {progress.bar()}"
                ],
            }
        return entries

    def _breakdown_progress(self, tasks):
        primary, cmap = common.list_rows_meta(tasks)
        task2children = collections.defaultdict(list)
        pk2task = {}
        for task in tasks.rows:
            parent = task.entries[cmap[schema.Fields.PrimaryGoal]].formatted
            pk = task.entries[primary].formatted
            task2children[parent].append(task)
            pk2task[pk] = task
        # Explicit breakdowns
        breakdown_tasks = [
            task.entries[primary].formatted for task in tasks.rows if
            task.entries[cmap[schema.Fields.Indirect]].formatted == 'breakdown'
        ]
        project_tasks = [
            task.entries[primary].formatted for task in tasks.rows
            if task.entries[cmap[schema.Fields.Action]].formatted == 'Work'
            and task.entries[cmap[
                schema.Fields.Indirect]].formatted == 'regularity'
        ]
        breakdowns = [
            pk2task[pk].entries[cmap[schema.Fields.Direct]].formatted
            for pk in breakdown_tasks
        ]
        # TODO if a path-to-match could support multiple values then we could
        # query all projects in parallel instead of in separate requests
        for project_task in project_tasks:
            project = pk2task[project_task].entries[cmap[
                schema.Fields.Direct]].formatted
            resp = self._query_project_workstreams(project)
            primary = common.get_primary(resp)
            for row in resp.rows:
                breakdowns.append(row.entries[primary].formatted)
        all_children = self._query_workstream_children(breakdowns)
        noun_progress = {
            breakdown: ProgressAmount()
            for breakdown in breakdowns
        }
        noun_primary, noun_cmap = common.list_rows_meta(all_children)
        for child in all_children.rows:
            parent = child.entries[noun_cmap[schema.Fields.Parent]].formatted
            status = child.entries[noun_cmap[schema.Fields.Status]].formatted
            noun_progress[parent].total += 1
            if status not in schema.active_statuses():
                noun_progress[parent].progress += 1
        # For project plans we go based on the status of the noun. For explicit breakdowns
        # we go based on the status of the tasks.
        progresses = {}
        for project_task in project_tasks:
            project = pk2task[project_task].entries[cmap[
                schema.Fields.Direct]].formatted
            progresses[project] = noun_progress[project]
        for breakdown_task in breakdown_tasks:
            children = task2children[breakdown_task]
            progress = len(
                set([
                    child.entries[cmap[schema.Fields.Direct]].formatted
                    for child in children
                    if child.entries[cmap[schema.Fields.Status]].formatted
                    not in schema.active_statuses()
                ]))
            breakdown = pk2task[breakdown_task].entries[cmap[
                schema.Fields.Direct]].formatted
            progresses[breakdown_task] = ProgressAmount(
                progress, noun_progress[breakdown].total)
        return progresses

    def _incremental_progress(self, tasks):
        primary, cmap = common.list_rows_meta(tasks)
        incrementals = [
            task.entries[primary].formatted for task in tasks.rows
            if task.entries[cmap[schema.Fields.Indirect]].formatted ==
            'incrementality'
        ]
        task2children = collections.defaultdict(list)
        pk2task = {}
        for task in tasks.rows:
            parent = task.entries[cmap[schema.Fields.PrimaryGoal]].formatted
            pk = task.entries[primary].formatted
            task2children[parent].append(task)
            pk2task[pk] = task
        progress = {
            incremental: ProgressAmount()
            for incremental in incrementals
        }
        for incremental in incrementals:
            task = pk2task[incremental]
            children = task2children[incremental]
            params = task.entries[cmap[schema.Fields.Parameters]].formatted
            param_values = eval(f"dict({params})")
            task_values = list(
                range(param_values['start'], param_values['end'] + 1,
                      param_values['delta']))
            max_i = 0
            for child in children:
                params = child.entries[cmap[
                    schema.Fields.Parameters]].formatted
                if 'fmt' in param_values:
                    cur_index = param_values['fmt'].split(" ").index("{cur}")
                    cur_value = int(params.split(" ")[cur_index])
                else:
                    cur_value = int(params)
                if cur_value not in task_values:
                    continue
                index_of_child = task_values.index(cur_value)
                max_i = max(max_i, index_of_child)
            progress[incremental] = ProgressAmount(max_i + 1, len(task_values))
        return progress

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
                            equal_match=jql_pb2.EqualMatch(
                                value=schema.Values.StatusHabitual),
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

    def percentage(self):
        return int(self.progress * 100 / self.total)

    def bar(self):
        blocks = self.percentage() // 4
        bar = "█" * blocks
        bar += " " * (25 - blocks)
        bar += "|"
        return bar
