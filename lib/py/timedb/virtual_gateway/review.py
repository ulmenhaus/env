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
        # TODO all entries need attention cycles
        ancestor_pk = self._query_active_cycle_pk()
        descendants = self._query_descendants(ancestor_pk)
        primary, cmap = common.list_rows_meta(descendants)
        task_pks = [row.entries[primary].formatted for row in descendants.rows]
        fields, _ = common.get_fields_for_items(self.client,
                                                schema.Tables.Tasks, task_pks)
        rows = {}
        progresses = self._breakdown_progress(descendants)
        progresses.update(self._incremental_progress(descendants))
        rows.update(self._workstream_entries(descendants, progresses))
        rows.update(self._projects_and_goals(descendants, progresses, fields))
        rows.update(self._habit_entries(descendants))
        rows.update(self._stat_entries(descendants, fields))
        return common.list_rows('vt.review', rows, request)

    def _stat_entries(self, tasks, fields):
        primary, cmap = common.list_rows_meta(tasks)
        captured_fields = [
            "Motivation", "Source", "Towards", "Area", "Genre", "Attendee"
        ]
        task_fields = [schema.Fields.Action, schema.Fields.Direct]
        attention_cycles = [
            task for task in tasks.rows
            if task.entries[cmap[schema.Fields.Action]].formatted == 'Attend'
            and task.entries[cmap[schema.Fields.Direct]].formatted == ''
        ]
        task2children = collections.defaultdict(list)
        for task in tasks.rows:
            parent = task.entries[cmap[schema.Fields.PrimaryGoal]].formatted
            task2children[parent].append(task)
        domain2stats = {}
        for attention_cycle in attention_cycles:
            stats = collections.defaultdict(collections.Counter)
            domain = attention_cycle.entries[cmap[schema.Fields.Indirect]].formatted
            domain2stats[domain] = stats
            pk = attention_cycle.entries[primary].formatted
            initiatives = task2children[pk]
            for initiative in initiatives:
                initiative_pk = initiative.entries[primary].formatted
                count = len(task2children[initiative_pk]) or 1
                if self._exclude_initiative_from_stats(initiative, fields[initiative_pk], cmap):
                    continue
                for field in captured_fields:
                    values = fields[initiative_pk][field] or ["None"]
                    for value in values:
                        stats[field][value] += count
                for field in task_fields:
                    value = initiative.entries[cmap[field]].formatted or "None"
                    stats[field][value] += count
        # TODO populate stats with all possible values (e.g. from vt.practices)
        rows = {}
        pk = 0
        for domain, stats in domain2stats.items():
            for by, values in stats.items():
                total = sum(values.values())
                for value, count in values.items():
                    as_progress = ProgressAmount(count, total)
                    pk += 1
                    rows[str(pk)] = {
                        "_pk": [str(pk)],
                        "A Domain": [domain],
                        "A Class": ["Stat"],
                        "By": [by],
                        "Description": [value],
                        "Number": [str(count)],
                        "Total": [str(total)],
                        "Z %%": [
                            f"{(str(as_progress.percentage()) + '%%').ljust(5)} {as_progress.bar()}"
                        ],
                    }
        return rows

    def _exclude_initiative_from_stats(self, task, task_fields, cmap):
        for cls in task_fields["Class"]:
            # Goals don't by themselves map to any time spent on work so are excluded
            # in calculating stats on time
            if cls == "Goal":
                return True
        if task.entries[cmap[schema.Fields.Indirect]].formatted == "regularity":
            # Habits are counted in habit entries so they aren't useful here
            return True
        return False

    def _habit_entries(self, tasks):
        primary, cmap = common.list_rows_meta(tasks)
        habit_tasks = [
            task for task in tasks.rows if task.entries[cmap[
                schema.Fields.Indirect]].formatted == 'regularity'
        ]
        task2children = collections.defaultdict(list)
        for task in tasks.rows:
            parent = task.entries[cmap[schema.Fields.PrimaryGoal]].formatted
            task2children[parent].append(task)
        rows = {}
        for habit in habit_tasks:
            habit_pk = habit.entries[primary].formatted
            children = [task for task in task2children[habit_pk]]
            successes = len([
                child for child in children
                if child.entries[cmap[schema.Fields.Status]].formatted ==
                schema.Values.StatusSatisfied
            ])
            total = len([
                child for child in children
                if child.entries[cmap[schema.Fields.Status]].formatted not in
                schema.active_statuses()
            ])
            success_rate = ((successes * 100) // total) if total > 0 else 100
            success_rate_str = f"\033[32m{success_rate}%%\033[0m"  # green colored
            if success_rate < 100:
                # TODO would be good to color this based on a custom value for expected success rate
                success_rate_str = f"\033[31m{success_rate}%%\033[0m"  # red colored
            rows[habit_pk] = {
                "_pk": [habit_pk],
                "A Class": ["Habit"],
                # TODO full pk not necessary here - remove params, span, indirect
                "Description": [habit_pk],
                "Successes": [str(successes)],
                "Total": [str(total)],
                # TODO would be good to support a "hit rate" based on how frequently the habit
                # is expected to happen
                "Z Success Rate": [success_rate_str],
            }
        return rows

    def _projects_and_goals(self, tasks, progresses, fields):
        primary, cmap = common.list_rows_meta(tasks)
        project2workstreams = collections.defaultdict(list)
        goal2projects = collections.defaultdict(list)
        for pk, attrs in fields.items():
            if '@timedb:Project:' in attrs['Class']:
                project2workstreams[pk]
                for workstream in fields[pk]['Workstream']:
                    if common.is_foreign(workstream) and common.parse_foreign(
                            workstream)[0] == schema.Tables.Tasks:
                        project2workstreams[pk].append(
                            common.parse_foreign(workstream)[1])
                for goal in fields[pk]['Goal']:
                    if common.is_foreign(goal) and common.parse_foreign(
                            goal)[0] == schema.Tables.Tasks:
                        goal2projects[common.parse_foreign(goal)[1]].append(pk)
            if '@timedb:Goal:' in attrs['Class']:
                goal2projects[pk]
        entries = {}
        project2progress = {pk: ProgressAmount() for pk in project2workstreams}
        for project_pk, workstreams in project2workstreams.items():
            for workstream in workstreams:
                progress = progresses[workstream]
                project2progress[project_pk].progress += progress.progress
                project2progress[project_pk].total += progress.total

        # TODO also display plans in this list
        for project_pk, progress in project2progress.items():
            entries[project_pk] = {
                "_pk": [project_pk],
                "A Class": ["Project"],
                # TODO full pk not necessary here - remove params and span
                "Description": [project_pk],
                "Progress": [str(progress.progress)],
                "Total": [str(progress.total)],
                "Z %%": [
                    f"{(str(progress.percentage()) + '%%').ljust(5)} {progress.bar()}"
                ],
            }
        for goal_pk, project_pks in goal2projects.items():
            progress = ProgressAmount()
            for project_pk in project_pks:
                project_progress = project2progress[project_pk]
                progress.progress = project_progress.progress
                progress.total += project_progress.total
            entries[goal_pk] = {
                "_pk": [goal_pk],
                "A Class": ["Goal"],
                # TODO full pk not necessary here - remove params and span
                "Description": [goal_pk],
                "Progress": [str(progress.progress)],
                "Total": [str(progress.total)],
                "Z %%": [
                    f"{(str(progress.percentage()) + '%%').ljust(5)} {progress.bar()}"
                ],
            }
        return entries

    def _workstream_entries(self, tasks, progresses):
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
                "A Class": ["Workstream"],
                # TODO technically I need to include any prepositions from the action
                # so should use the pk library, but this is good enough for most purposes
                "Description": [entry_description],
                "Progress": [str(progress.progress)],
                "Total": [str(progress.total)],
                "Z %%": [
                    f"{(str(progress.percentage()) + '%%').ljust(5)} {progress.bar()}"
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
        plan_tasks = [
            task.entries[primary].formatted for task in tasks.rows
            if task.entries[cmap[schema.Fields.Action]].formatted == 'Work'
            and task.entries[cmap[
                schema.Fields.Indirect]].formatted == 'regularity'
        ]
        breakdowns = [
            pk2task[pk].entries[cmap[schema.Fields.Direct]].formatted
            for pk in breakdown_tasks
        ]
        plan_workstreams = []
        # TODO if a path-to-match could support multiple values then we could
        # query all plans in parallel instead of in separate requests
        for plan_task in plan_tasks:
            plan = pk2task[plan_task].entries[cmap[
                schema.Fields.Direct]].formatted
            resp = self._query_plan_workstreams(plan)
            primary = common.get_primary(resp)
            for row in resp.rows:
                plan_workstreams.append(row.entries[primary].formatted)
        breakdowns.extend(plan_workstreams)
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
        # For plans we go based on the status of the noun. For explicit breakdowns
        # we go based on the status of the tasks.
        progresses = {}
        for plan_workstream in plan_workstreams:
            progresses[plan_workstream] = noun_progress[plan_workstream]
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
            end_delta = 1 if param_values['start'] < param_values['end'] else -1
            task_values = list(
                range(param_values['start'], param_values['end'] + end_delta,
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

    def _query_plan_workstreams(self, plan_pk):
        return self.client.ListRows(
            jql_pb2.ListRowsRequest(
                table=schema.Tables.Nouns,
                conditions=[
                    jql_pb2.Condition(requires=[
                        jql_pb2.Filter(
                            column=schema.Fields.Parent,
                            path_to_match=jql_pb2.PathToMatch(value=plan_pk),
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
        if self.total == 0:
            return 100
        return int(self.progress * 100 / self.total)

    def bar(self):
        blocks = self.percentage() // 4
        bar = "█" * blocks
        bar += " " * (25 - blocks)
        bar += "|"
        return bar
