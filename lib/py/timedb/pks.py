import datetime

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


class TimeDB(object):

    def __init__(self, db):
        self.db = db

    def update_files_pk(self, old, new):
        files = self.db["files"]
        f = files[old]
        del files[old]
        files[new] = f

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
        full_id = "tasks {}".format(old)
        new_full_id = "tasks {}".format(new)
        # Take a snapshot of assertions to not modify while iterating
        for pk, assn in list(self.db["assertions"].items()):
            if assn["Arg1"] == full_id:
                assn["Arg1"] = new_full_id
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
    key = (assn["A Relation"], assn["Arg0"], assn["Order"])
    return str(key)
