import collections

from jql import jql_pb2
from timedb import pks, schema


class CycleManager(object):
    def __init__(self, db):
        self.db = db

    def add_cycle_for(self, pk):
        root_task_pk = self.find_root_task()
        tasks = self.db['tasks']
        lineage = self.construct_lineage(pk)
        ancestry = list(reversed(lineage))[1:]
        current_cycles = self.collect_attention_cycles(
            root_task_pk)  # map from noun to task pk
        parent_task_pk = root_task_pk
        # find the most specific attention cycle that matches this one
        for ancestor in ancestry:
            if ancestor in current_cycles:
                parent_task_pk = current_cycles[ancestor]
                break

        if tasks[parent_task_pk]['Indirect'] == "":
            # if this is the root task for this cycle
            if lineage[:1] != ["root"]:
                raise ValueError(
                    "attention cycles can only be automatically created for models:", pk
                )
            if lineage[1] != pk:
                # create a cycle for root's grandchild and then add this cycle to it
                self.add_cycle_for(lineage[1])
                return self.add_cycle_for(pk)
        new_task = dict(tasks[root_task_pk])
        new_task['Action'] = "Attend"
        new_task['Direct'] = ""
        new_task['Indirect'] = pk
        new_task['Status'] = 'Habitual'
        new_task['Primary Goal'] = parent_task_pk
        new_task_pk = pks.pk_for_task(new_task, self.db['actions'])
        tasks[new_task_pk] = new_task

        children = [
            pk for pk, task in tasks.items()
            if task['Primary Goal'] == parent_task_pk
        ]
        # will store the next closest ancestor for each child at this level
        child2next = {}
        this_level = tasks[parent_task_pk]['Indirect']
        if this_level == "":
            # we never reorganize the root for this cycle as its children should all be grandchildren
            # of the root noun
            return new_task_pk

        # first reparent any sibling that is actually a child in case we've intentionally added
        # an intermediate goal
        for child_pk in children:
            task = tasks[child_pk]
            if task['Action'] != "Attend" or task['Direct'] != "":
                continue
            noun_ancestry = self.construct_lineage(task['Indirect'])[:-1]
            if pk in noun_ancestry:
                task['Primary Goal'] = new_task_pk
            else:
                lineage = self.construct_lineage(task['Indirect'])
                if this_level == '':
                    # we are at the root task for this cycle so go with the grandchild of root
                    next_ancestor = lineage[2]
                else:
                    next_ancestor = lineage[lineage.index(this_level) + 1]
                if next_ancestor != task['Indirect']:
                    # for grouping purposes we don't care about tasks that are already a child of our parent
                    child2next[child_pk] = next_ancestor
        counts = collections.Counter(child2next.values())
        if len(counts) == 1:
            # if every child has the same next ancestor then we don't want to group further
            return new_task_pk
        # otherwise for any common next ancestor that appears more than once add it as a cycle
        for next_ancestor, count in counts.items():
            if count > 1:
                self.add_cycle_for(next_ancestor)
        return new_task_pk

    def find_root_task(self):
        tasks = self.db['tasks']
        is_candidate = lambda task: (task['Action'] == "Accomplish" and task[
            'Direct'] == "set goals" and task['Indirect'] == "")
        # pending task takes precedance as it means we're planning our
        # next set of goals
        for pk, task in tasks.items():
            if is_candidate(task) and task['Status'] == "Pending":
                return pk
        # fallback to active task as we are executing and want to add a cycle
        for pk, task in tasks.items():
            if is_candidate(task) and task['Status'] == "Habitual":
                return pk

    def construct_lineage(self, pk):
        nouns = self.db['nouns']
        noun = nouns[pk]
        lineage = [pk]
        while noun['Parent'] != "":
            pk = noun['Parent']
            noun = nouns[pk]
            if pk in lineage:
                raise ValueError("Cycle detected in lineage for",
                                 pk)  # not original pk but it doesn't matter
            lineage.insert(0, pk)
        return lineage

    def collect_attention_cycles(self, root_task):
        d = {}
        node2children = collections.defaultdict(list)
        tasks = self.db['tasks']
        for pk, task in tasks.items():
            node2children[task['Primary Goal']].append(pk)

        # sometimes we break attention cycles into samenamed cycles of smaller cadence,
        # but we only want the big ones
        is_ac = lambda task: (task['Action'] == "Attend" and task['Direct'] ==
                              "" and task['Param~Span'] == tasks[root_task][
                                  'Param~Span'])
        queue = [
            child for child in node2children[root_task] if is_ac(tasks[child])
        ]
        while queue:
            elem = queue.pop()
            task = tasks[elem]
            if task['Indirect'] in d:
                raise ValueError("cycle detected among attention cycles", elem)
            d[task['Indirect']] = elem
            queue.extend([
                child for child in node2children[elem] if is_ac(tasks[child])
            ])
        return d

def add_task_from_template(dbms, table, pk):
    resp = dbms.GetRow(jql_pb2.GetRowRequest(table=table, pk=pk))
    cmap = {c.name: i for i, c in enumerate(resp.columns)}
    parent = ""
    if schema.Fields.Parent in cmap:
        parent = resp.row.entries[cmap[schema.Fields.Parent]].formatted
    if not parent:
        # Domain references an attention cycle
        domain_pk = resp.row.entries[cmap[schema.Fields.Domain]].formatted
        parent_resp = dbms.ListRows(jql_pb2.ListRowsRequest(
            table=schema.Tables.Tasks,
            conditions=[
                jql_pb2.Condition(requires=[
                    jql_pb2.Filter(column=schema.Fields.Action, equal_match=jql_pb2.EqualMatch(value="Attend")),
                    jql_pb2.Filter(column=schema.Fields.Indirect, equal_match=jql_pb2.EqualMatch(value=domain_pk)),
                    jql_pb2.Filter(column=schema.Fields.Status, equal_match=jql_pb2.EqualMatch(value=schema.Values.StatusHabitual)),
                ]),
            ],
        ))
        parent_cmap = {c.name: i for i, c in enumerate(parent_resp.columns)}
        primary_ix, = [i for i, c in enumerate(parent_resp.columns) if c.primary]
        parent = parent_resp.rows[0].entries[primary_ix].formatted
    fields = {
        schema.Fields.Action: resp.row.entries[cmap[schema.Fields.Action]].formatted,
        schema.Fields.Direct: resp.row.entries[cmap[schema.Fields.Direct]].formatted,
        schema.Fields.PrimaryGoal: parent,
        schema.Fields.ParamStart: "",
        schema.Fields.Status: schema.Values.StatusActive,
    }
    dbms.WriteRow(jql_pb2.WriteRowRequest(
        table=schema.Tables.Tasks,
        pk=pk, # temporarily set the pk to match the pk from the original table
        fields=fields,
        insert_only=True,
    ))
    # set the pk after the fact so the jql daemon will first
    # set the date to today
    setter = pks.PKSetter(dbms)
    new_pk = setter.update_task(pk)
    
    # finally add the attributes
    attrs = {
        schema.Fields.Motivation: resp.row.entries[cmap[schema.Fields.Motivation]].formatted,
        schema.Fields.Source: resp.row.entries[cmap[schema.Fields.Source]].formatted,
        schema.Fields.Towards: resp.row.entries[cmap[schema.Fields.Towards]].formatted,
    }
    for key, value in attrs.items():
        fields = {
            schema.Fields.Relation: f".{key}",
            schema.Fields.Arg0: f"{schema.Tables.Tasks} {new_pk}",
            schema.Fields.Arg1: value,
            schema.Fields.Order: "0",
        }
        assn_pk = pks.pk_for_assertion(fields)
        dbms.WriteRow(jql_pb2.WriteRowRequest(
            table=schema.Tables.Assertions,
            pk=assn_pk,
            fields=fields,
        ))
