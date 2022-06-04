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
    direct_clause, indirect_clause = "", ""
    mandate = [action, prepreposition, task['Direct'], preposition, task['Indirect']]
    if task["Parameters"]:
        marker = " at" if action in ("Extend", "Improve", "Sustain") else ","
        mandate.append("{} {}".format(marker, task['Parameters']))
    planned_start, planned_span = task["Param~Start"], task["Param~Span"]
    # TODO can probably get rid of the dependency on parent now that breakdown
    # tasks are simpler (just use the same span and start as their parent and
    # then rely on log entries to track when they were started and done)
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
    mandate.append(" ({})".format(distinguisher))
    return mandate


def pk_for_task(task, parent):
    return "".join(pk_terms_for_task(task, parent))
