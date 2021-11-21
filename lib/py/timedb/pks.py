import datetime


def pk_for_task(task, parent):
    action = task['Action']
    preposition = "with"
    prepreposition = ""
    if action in ("Attend", "Migrate", "Transfer", "Travel"):
        preposition = "to"
    elif action in ("Present", ):
        preposition = "on"
    elif action in ("Buy", ):
        preposition = "for"
    elif action in (
            "Ideate",
            "Deliberate",
    ):
        preposition = "on"
    elif action in ("Upload", ):
        preposition = "from"
    elif action in ("Vacation", ):
        prepreposition = "in"
    elif action in ("Lunch", "Dine", "Shop", "Tennis"):
        prepreposition = "at"
    direct_clause, indirect_clause = "", ""
    if task['Direct']:
        direct_clause = " {}".format(task['Direct'])
        if prepreposition:
            direct_clause = " {}{}".format(prepreposition, direct_clause)
    if task['Indirect']:
        indirect_clause = " {} {}".format(preposition, task['Indirect'])
    mandate = "{}{}{}".format(action, direct_clause, indirect_clause)
    if task["Parameters"]:
        marker = " at" if action in ("Extend", "Improve", "Sustain") else ","
        mandate += "{} {}".format(marker, task['Parameters'])
    planned_start, planned_span = task["Param~Start"], task["Param~Span"]
    if parent.get("Indirect") == "breakdown":
        planned_start, planned_span = parent["Param~Start"], parent[
            "Param~Span"]
    distinguisher = (
        datetime.datetime(1969, 12, 31) +
        datetime.timedelta(days=int(planned_start))).strftime("%d %b %Y")
    # TODO need to use parent span here but will do once we can edit the table
    # to match
    if task["Param~Span"] and task["Param~Span"] != "Day":
        distinguisher = "{} of {}".format(task["Param~Span"], distinguisher)
    mandate += " ({})".format(distinguisher)
    return mandate
