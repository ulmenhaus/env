import datetime

from jql import jql_pb2
from timedb import schema

HABIT_MODES = ("breakdown", "consistency", "continuity", "habituality",
               "incrementality", "regularity")


def class_for_task(action, task):
    return action['Class']


def pk_terms_for_task(task, actions):
    action_name, direct, indirect = task['Action'], task['Direct'], task[
        'Indirect']
    # Legacy behavior for actions that don't yet exist in the actions table
    # TODO we can get rid of this legacy behavior once we migrate
    # all actions to use the new actions table
    prepreposition = " " if direct else ""
    preposition = " with " if indirect else ""
    if action_name in actions:
        action = actions[action_name]
        direct_parts = action['Direct'].split(" ")
        indirect_parts = action['Indirect'].split(" ")
        if direct and len(direct_parts) > 1:
            prepreposition = f" {direct_parts[0]} "
        if indirect and len(indirect_parts) > 1:
            preposition = f" {indirect_parts[0]} "

    if indirect in HABIT_MODES:
        preposition = " with "
    mandate = [
        action_name, prepreposition, task['Direct'], preposition,
        task['Indirect']
    ]
    if task["Parameters"]:
        marker = " at" if action_name in ("Extend", "Improve",
                                          "Sustain") else ","
        mandate.append("{} {}".format(marker, task['Parameters']))
    planned_start, planned_span = task["Param~Start"], task["Param~Span"]
    distinguisher = (
        datetime.datetime(1970, 1, 1) +
        datetime.timedelta(days=int(planned_start))).strftime("%d %b %Y")
    if task["Param~Span"] and task["Param~Span"] != "Day":
        distinguisher = "{} of {}".format(task["Param~Span"], distinguisher)
    mandate.append(" ({})".format(distinguisher))
    return mandate


def pk_for_task(task, actions):
    return "".join(pk_terms_for_task(task, actions))


def pk_for_noun(noun):
    modifier, description, disambiguator = noun['A Modifier'], noun[
        'Description'], noun['Disambiguator']
    idn = description
    if modifier:
        idn = f"{modifier} {idn}"
    if disambiguator:
        if modifier and disambiguator == "approach":
            idn = f"{modifier} approach to {description}"
        elif modifier and disambiguator == "concept":
            idn = f"{modifier} concept of {description}"
        else:
            idn = f"{idn} ({disambiguator})"
    ctx, cnl = noun['Context'], noun['Coordinal']
    # we only consider the coordinal of a noun as part of its identity once we are committed to it
    if cnl != "" and noun['Relation'] == "Item" and noun['Status'] not in [
            'Idea', 'Pending', 'Someday'
    ]:
        idn = f"[{ctx}][{cnl}] {idn}" if ctx else f"[{cnl}] {idn}"
    elif ctx != "":
        idn = f"[{ctx}] {idn}"
    return idn


# TODO the PKSetter reimplements this interface for v2 macros. Once v1 macros are deprecated we can
# remove this class
class TimeDB(object):

    def __init__(self, db):
        self.db = db
        self.noun_to_context = {
            attrs['Parent']: code
            for code, attrs in self.db['contexts'].items()
        }

    def update_files_pk(self, old, new):
        files = self.db["files"]
        f = files[old]
        del files[old]
        files[new] = f

    def update_noun(self, old):
        noun = self.db['nouns'][old]
        noun['Context'] = self.noun_to_context.get(noun['Parent'], "")
        if not noun['Description']:
            noun['Description'] = old
        new = pk_for_noun(noun)
        if old == new:
            return
        if new in self.db['nouns']:
            raise ValueError("key already exists", new)
        del self.db['nouns'][old]
        self.db["nouns"][new] = noun
        self.update_arg_in_assertions("nouns", old, new)
        if old == '':
            return
        for noun in self.db['nouns'].values():
            if noun['Parent'] == old:
                noun['Parent'] = new
        affected = [
            task_pk for task_pk, task in self.db['tasks'].items()
            if old in [task['Direct'], task['Indirect']]
        ]
        for task_pk in affected:
            task = self.db['tasks'][task_pk]
            if task['Direct'] == old:
                task['Direct'] = new
            if task['Indirect'] == old:
                task['Indirect'] = new
            self.update_task(task_pk)

    def update_task(self, pk):
        task = self.db['tasks'][pk]
        return self.update_task_pk(pk, pk_for_task(task, self.db['actions']))

    def update_task_pk(self, old, new):
        if old == new:
            return
        if new in self.db["tasks"]:
            raise ValueError("key already exists", new)
        task = self.db["tasks"][old]
        del self.db["tasks"][old]
        self.db["tasks"][new] = task
        self.update_task_in_log(old, new)
        self.update_arg_in_assertions("tasks", old, new)
        if old == '':
            return
        for task in self.db["tasks"].values():
            if task["Primary Goal"] == old:
                task["Primary Goal"] = new

    def update_task_in_log(self, old, new):
        # TODO should hash this
        for pk, log in self.db["log"].items():
            if log["A Task"] == old:
                log["A Task"] = new
            # NOTE not changing PKs here as they require context on
            # other entries and it's not really needed

    def update_arg_in_assertions(self, table, old, new):
        full_id = "{} {}".format(table, old)
        new_full_id = "{} {}".format(table, new)
        # Take a snapshot of assertions to not modify while iterating
        for pk, assn in list(self.db["assertions"].items()):
            if table == "nouns" and f"@timedb:{old}:" in assn["Arg1"]:
                assn["Arg1"] = assn["Arg1"].replace(f"@timedb:{old}:",
                                                    f"@timedb:{new}:")
            if assn["Arg0"] == full_id:
                assn["Arg0"] = new_full_id
                new_pk = pk_for_assertion(assn)
                del self.db["assertions"][pk]
                self.db["assertions"][new_pk] = assn
            if assn["A Relation"] == ".Do Today" and assn[
                    "Arg1"] == f"[ ] {old}":
                assn["Arg1"] = f"[ ] {new}"
            if assn["A Relation"] == ".Do Today" and assn[
                    "Arg1"] == f"[x] {old}":
                assn["Arg1"] = f"[x] {new}"


def pk_for_assertion(assn):
    key = (assn["A Relation"], assn["Arg0"], str(assn["Order"]).zfill(4))
    return str(key)


class PKSetter(object):

    def __init__(self, dbms):
        self.dbms = dbms
        self.actions = None
        self.parent_to_context = None

    def _populate_actions(self):
        if self.actions is not None:
            return
        request = jql_pb2.ListRowsRequest(table=schema.Tables.Actions)
        response = self.dbms.ListRows(request)
        actions = _protos_to_dict(response.columns, response.rows)
        self.actions = actions

    def _populate_contexts(self):
        if self.parent_to_context is not None:
            return
        request = jql_pb2.ListRowsRequest(table=schema.Tables.Contexts)
        response = self.dbms.ListRows(request)
        contexts = _protos_to_dict(response.columns, response.rows)
        self.parent_to_context = {}
        for context in contexts.values():
            self.parent_to_context[context[schema.Fields.Parent]] = context[
                schema.Fields.Code]

    def update_noun(self, old):
        self._populate_contexts()
        request = jql_pb2.GetRowRequest(table=schema.Tables.Nouns, pk=old)
        response = self.dbms.GetRow(request)
        noun = _proto_to_dict(response.columns, response.row)
        noun[schema.Fields.Context] = self.parent_to_context.get(
            noun[schema.Fields.Parent], "")
        if not noun[schema.Fields.Description]:
            noun[schema.Fields.Description] = old
        new = pk_for_noun(noun)
        noun[schema.Fields.Identifier] = new
        possibly_changed = [
            schema.Fields.Identifier, schema.Fields.Description,
            schema.Fields.Context
        ]
        update_request = jql_pb2.WriteRowRequest(
            table=schema.Tables.Nouns,
            pk=old,
            fields={k: noun[k]
                    for k in possibly_changed},
            update_only=True,
        )
        self.dbms.WriteRow(update_request)
        if old == new:
            return
        self._update_all(schema.Tables.Nouns, schema.Fields.Parent, old, new)
        self._update_all(
            schema.Tables.Assertions,
            schema.Fields.Arg0,
            f"{schema.Tables.Nouns} {old}",
            f"{schema.Tables.Nouns} {new}",
        )
        self._update_all(schema.Tables.Assertions,
                         schema.Fields.Arg1,
                         f"@timedb:{old}:",
                         f"@timedb:{new}:",
                         exact=False)
        self._update_all(schema.Tables.Tasks, schema.Fields.Direct, old, new)
        self._update_all(schema.Tables.Tasks, schema.Fields.Indirect, old, new)

    def update_task(self, old):
        self._populate_actions()
        request = jql_pb2.GetRowRequest(table=schema.Tables.Tasks, pk=old)
        response = self.dbms.GetRow(request)
        new = pk_for_task(_proto_to_dict(response.columns, response.row),
                          self.actions)
        if old == new:
            return
        update_request = jql_pb2.WriteRowRequest(
            table=schema.Tables.Tasks,
            pk=old,
            fields={schema.Fields.UDescription: new},
            update_only=True,
        )
        self.dbms.WriteRow(update_request)
        self._update_all(schema.Tables.Tasks, schema.Fields.PrimaryGoal, old,
                         new)
        self._update_all(
            schema.Tables.Assertions,
            schema.Fields.Arg0,
            f"{schema.Tables.Tasks} {old}",
            f"{schema.Tables.Tasks} {new}",
        )
        self._update_all(
            schema.Tables.Assertions,
            schema.Fields.Arg1,
            f"[ ] {old}",
            f"[ ] {new}",
        )
        self._update_all(
            schema.Tables.Assertions,
            schema.Fields.Arg1,
            f"[x] {old}",
            f"[x] {new}",
        )
        # TODO update the log table

    def update_assertion(self, old):
        request = jql_pb2.GetRowRequest(table=schema.Tables.Assertions, pk=old)
        response = self.dbms.GetRow(request)
        new = pk_for_assertion(_proto_to_dict(response.columns, response.row))
        update_request = jql_pb2.WriteRowRequest(
            table=schema.Tables.Assertions,
            pk=old,
            fields={schema.Fields.UDescription: new},
            update_only=True,
        )
        self.dbms.WriteRow(update_request)

    def update(self, table, old):
        # TODO this first pass implementation needs full parity with the old implementation
        # 1. Support for contexts
        # 3. Updates in day planner
        if table == schema.Tables.Nouns:
            self.update_noun(old)
        elif table == schema.Tables.Tasks:
            self.update_task(old)
        elif table == schema.Tables.Assertions:
            self.update_assertion(old)
        else:
            raise ValueError("Setting PK not supported for table", table)

    def _update_all(self, table, field, old, new, exact=True, recursive=True):
        requires = jql_pb2.Filter(
            column=field, contains_match=jql_pb2.ContainsMatch(value=old))
        if exact:
            requires = jql_pb2.Filter(
                column=field, equal_match=jql_pb2.EqualMatch(value=old))
        query = jql_pb2.ListRowsRequest(
            table=table,
            conditions=[
                jql_pb2.Condition(requires=[requires]),
            ],
        )
        response = self.dbms.ListRows(query)
        colix, = [i for i, c in enumerate(response.columns) if c.name == field]
        primary_ix, = [i for i, c in enumerate(response.columns) if c.primary]
        for row in response.rows:
            if exact:
                update_request = jql_pb2.WriteRowRequest(
                    table=table,
                    pk=row.entries[primary_ix].formatted,
                    fields={field: new},
                    update_only=True,
                )
            else:
                updated = row.entries[colix].formatted.replace(old, new)
                update_request = jql_pb2.WriteRowRequest(
                    table=table,
                    pk=row.entries[primary_ix].formatted,
                    fields={field: updated},
                    update_only=True,
                )
            self.dbms.WriteRow(update_request)
            if recursive:
                self.update(table, row.entries[primary_ix].formatted)


def _proto_to_dict(columns, row):
    d = {}
    for i, col in enumerate(columns):
        if col.type == jql_pb2.EntryType.DATE:
            parsed = datetime.datetime.strptime(row.entries[i].formatted,
                                                "%d %b %Y")
            delta = parsed - datetime.datetime(1970, 1, 1)
            d[col.name] = int(delta.days)
        elif col.type == jql_pb2.EntryType.INT:
            d[col.name] = int(row.entries[i].formatted)
        elif col.type == jql_pb2.EntryType.TIME:
            raise NotImplementedError(
                "conversion from time types not supported")
        else:
            d[col.name] = row.entries[i].formatted
    return d


def _protos_to_dict(columns, rows):
    ds = {}
    primary, = [i for i, c in enumerate(columns) if c.primary]
    for row in rows:
        ds[row.entries[primary].formatted] = _proto_to_dict(columns, row)
    return ds
