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

def pk_for_noun(noun):
    idn = noun['Description']
    ctx, cnl = noun['Context'], noun['Coordinal']
    # we only consider the coordinal of a noun as part of its identity once we are committed to it
    if cnl != "" and noun['Relation'] == "Item" and noun['Status'] not in ['Idea', 'Pending', 'Someday']:
        idn = f"[{ctx}][{cnl}] {idn}" if ctx else f"[{cnl}] {idn}"
    elif ctx != "":
        idn = f"[{ctx}] {idn}"
    return idn

class TimeDB(object):
    def __init__(self, db):
        self.db = db
        self.noun_to_context = {attrs['Parent']: code for code, attrs in self.db['contexts'].items()}

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
        affected = [task_pk for task_pk, task in self.db['tasks'].items() if old in [task['Direct'], task['Indirect']]]
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
            if assn["Arg1"] == full_id:
                # TODO update @timedb assertions
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
