import datetime


def pk_terms_for_task(task, parent):
    action = task['Action']
    preposition = " with " if task['Indirect'] else ""
    prepreposition = " " if task['Direct'] else ""
    if action == "Attend" and not task['Direct']:
        preposition = " to "
    if action in ("Migrate", "Transfer", "Travel"):
        preposition = " to "
    elif action in ("Present", ):
        preposition = " on "
    elif action in ("Buy", ):
        preposition = " for "
    elif action in (
            "Ideate",
            "Deliberate",
            "Muse",
    ):
        preposition = " on "
    elif action in ("Upload", ):
        preposition = " from "
    elif action in ("Vacation", ):
        prepreposition = " in "
    elif action in ("Lunch", "Dine", "Shop", "Tennis", "Coffee"):
        if task['Direct']:
            prepreposition = " at "
    elif action in ("Tend", ):
        prepreposition = " to "
    elif action in ("Jam", ):
        if task['Direct']:
            prepreposition = " at "
    elif action == "Liase":
        prepreposition = " with "
        preposition = " on "
    direct_clause, indirect_clause = "", ""
    mandate = [
        action, prepreposition, task['Direct'], preposition, task['Indirect']
    ]
    if task["Parameters"]:
        marker = " at" if action in ("Extend", "Improve", "Sustain") else ","
        mandate.append("{} {}".format(marker, task['Parameters']))
    planned_start, planned_span = task["Param~Start"], task["Param~Span"]
    distinguisher = (
        datetime.datetime(1970, 1, 1) +
        datetime.timedelta(days=int(planned_start))).strftime("%d %b %Y")
    if task["Param~Span"] and task["Param~Span"] != "Day":
        distinguisher = "{} of {}".format(task["Param~Span"], distinguisher)
    mandate.append(" ({})".format(distinguisher))
    return mandate


def pk_for_task(task, parent):
    # TODO(rabrams) this function no longer needs a provided parent
    # so let's get rid of it as an input
    return "".join(pk_terms_for_task(task, parent))


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
            if assn["A Relation"] == ".Do Today" and assn["Arg1"] == f"[ ] {old}":
                assn["Arg1"] = f"[ ] {new}"
            if assn["A Relation"] == ".Do Today" and assn["Arg1"] == f"[x] {old}":
                assn["Arg1"] = f"[x] {new}"



def pk_for_assertion(assn):
    key = (assn["A Relation"], assn["Arg0"], assn["Order"])
    return str(key)
