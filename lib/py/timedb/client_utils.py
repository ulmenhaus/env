import collections
import json

from datetime import datetime, timedelta

from jql import jql_pb2
from timedb import schema

_DATE_FORMAT = "%Y-%m-%d"


def list_rows_meta(resp):
    primary, = [i for i, c in enumerate(resp.columns) if c.primary]
    cmap = {c.name: i for i, c in enumerate(resp.columns)}
    return primary, cmap


def get_primary(response):
    return [i for i, c in enumerate(response.columns) if c.primary][0]


def parse_full_pk(full_pk):
    return full_pk.split(" ", 1)


def is_foreign(entry):
    # TODO technically we should look for colons in the middle, but some pks right now
    # have colons in the middle that should be escaped first
    if not entry:
        return True
    return entry.startswith("@{") and entry.endswith("}")


def strip_foreign(entry):
    return entry[len("@{"):-len("}")]


def strip_foreign_noun(entry):
    return entry[len("@{nouns "):-1]


def parse_foreign(entry):
    return parse_full_pk(strip_foreign(entry))


def full_pk(table, pk):
    return f"{table} {pk}"


def encode_pk(noun_pk, assn_pks):
    return "\t".join([noun_pk, json.dumps(assn_pks)])


def decode_pk(pk):
    noun_pk, assn_pks = pk.split("\t")
    return noun_pk, json.loads(assn_pks)


def is_encoded_pk(pk):
    return "\t" in pk


def is_date_foreign(entry):
    if not is_foreign(entry):
        return False
    table, _ = parse_foreign(entry)
    return table == schema.Tables.Dates


def increment_date_foreign(value, amount):
    _, date_str = parse_foreign(value)
    date = datetime.strptime(date_str, _DATE_FORMAT)
    new_date = date + timedelta(days=amount)
    return f"@{{{schema.Tables.Dates} {new_date.strftime(_DATE_FORMAT)}}}"


def parse_rating(pk):
    return map(int, pk.split(" "))


def format_attrs(attrs):
    if len(attrs) == 0:
        return ""
    if len(attrs) == 1:
        return attrs[0]
    return "\n".join(f"* {attr}" for attr in attrs)


def present_attrs(attrs):
    if len(attrs) == 0:
        return ""
    if len(attrs) == 1:
        entry = attrs[0]
        lines = entry.split("\n")
        if len(lines) > 1:
            return lines[0] + f" + {len(lines) - 1} lines"
        if is_foreign(entry):
            try:
                table, pk = parse_foreign(entry)
                if table == schema.Tables.Ratings:
                    num, denom = parse_rating(pk)
                    return "●" * num + "○" * (denom - num)
                return pk
            except:
                pass
        return entry
    return f"{len(attrs)} entries"


def link_attrs(attrs):
    if len(attrs) != 1:
        return ""
    attr, = attrs
    return strip_foreign(attr) if is_foreign(attr) else ""


def selected_target(request):
    for condition in request.conditions:
        for f in condition.requires:
            match_type = f.WhichOneof('match')
            if match_type == "equal_match" and f.column == '-> Item':
                return f.equal_match.value


def selected_targets(request, column='pk'):
    for condition in request.conditions:
        for f in condition.requires:
            match_type = f.WhichOneof('match')
            if match_type == "in_match" and f.column == column:
                return f.in_match.values


def get_fields_for_items(client, table, pks, fields=(), include_children=False):
    if include_children:
        raise NotImplementedError("including direct children in query for fields is not yet implemented")
    prefix = f"{table} " if table else ""
    ret = {pk: collections.defaultdict(list) for pk in pks}
    full_pks = [f"{prefix}{pk}" for pk in pks]
    filters = [
        jql_pb2.Filter(
            column=schema.Fields.Arg0,
            in_match=jql_pb2.InMatch(values=full_pks),
        ),
    ]

    if fields:
        filters.append(
            jql_pb2.Filter(
                column=schema.Fields.Relation,
                in_match=jql_pb2.InMatch(
                    values=[f".{field}" for field in fields]),
            ))
    assertions_request = jql_pb2.ListRowsRequest(
        table=schema.Tables.Assertions,
        conditions=[jql_pb2.Condition(requires=filters)],
        order_by=schema.Fields.Order,
    )
    assertions_response = client.ListRows(assertions_request)
    assn_cmap = {c.name: i for i, c in enumerate(assertions_response.columns)}
    assn_primary, = [
        i for i, c in enumerate(assertions_response.columns) if c.primary
    ]
    assn_pks = collections.defaultdict(lambda: collections.defaultdict(list))
    has_tasks = any([full_pk.startswith("tasks ") for full_pk in full_pks])
    if has_tasks:
        for task_pk, attributes in _implicit_task_attributes_from_params(
                client, full_pks).items():
            pk = task_pk.split(" ", 1)[1] if table else task_pk
            for k, v in attributes.items():
                ret[pk][k] = [v]
    for row in assertions_response.rows:
        rel = row.entries[assn_cmap[schema.Fields.Relation]].formatted
        value = row.entries[assn_cmap[schema.Fields.Arg1]].formatted
        arg0 = row.entries[assn_cmap[schema.Fields.Arg0]].formatted
        pk = arg0.split(" ", 1)[1] if table else arg0
        ret[pk][rel[1:]].append(value)
        assn_pks[pk][rel[1:]].append(
            [row.entries[assn_primary].formatted, value])
    return ret, assn_pks


def _implicit_task_attributes_from_params(client, full_pks):
    actions = client.ListRows(
        jql_pb2.ListRowsRequest(table=schema.Tables.Actions))
    action_primary, action_cmap = list_rows_meta(actions)
    actions_by_primary = {
        action.entries[action_primary].formatted: action
        for action in actions.rows
    }
    task_pks = [task for table, task in map(parse_full_pk, full_pks)]
    tasks = client.ListRows(
        jql_pb2.ListRowsRequest(
            table=schema.Tables.Tasks,
            conditions=[
                jql_pb2.Condition(requires=[
                    jql_pb2.Filter(
                        column=schema.Fields.UDescription,
                        in_match=jql_pb2.InMatch(values=task_pks),
                    ),
                ]),
            ],
        ))
    pk2attributes = {}
    primary, cmap = list_rows_meta(tasks)
    for task in tasks.rows:
        task_pk = task.entries[primary].formatted
        action_primary = task.entries[cmap[schema.Fields.Action]].formatted
        direct = task.entries[cmap[schema.Fields.Direct]].formatted
        indirect = task.entries[cmap[schema.Fields.Indirect]].formatted
        start = task.entries[cmap[schema.Fields.ParamStart]].formatted
        date = datetime.strptime(start, "%d %b %Y").strftime("%Y-%m-%d")
        full_pk_val = f"tasks {task_pk}"
        pk2attributes[full_pk_val] = {
            schema.Fields.Action: action_primary,
            "A Date": date,
        }
        if action_primary in actions_by_primary:
            action = actions_by_primary[action_primary]
            cls = action.entries[action_cmap[schema.Fields.Class]].formatted
            if schema.indirect_indicates_habit(indirect):
                pk2attributes[full_pk_val]["Class"] = "Habit"
            else:
                pk2attributes[full_pk_val]["Class"] = cls
            if direct:
                ps = action.entries[action_cmap[
                    schema.Fields.Direct]].formatted
                relation = schema.relation_from_parameter_schema(ps)
                pk2attributes[full_pk_val][relation] = f"@{{nouns {direct}}}"
            if indirect:
                ps = action.entries[action_cmap[
                    schema.Fields.Indirect]].formatted
                relation = schema.relation_from_parameter_schema(ps)
                pk2attributes[full_pk_val][relation] = f"@{{nouns {indirect}}}"
        else:
            if direct:
                pk2attributes[full_pk_val]["Direct"] = f"@{{nouns {direct}}}"
            if indirect:
                pk2attributes[full_pk_val]["Indirect"] = f"@{{nouns {indirect}}}"
    return pk2attributes


def get_todays_day_plan(client):
    """Returns the task PK of today's active day plan, or None if not found.

    A day plan is a task with Action=Plan, Direct=today, Indirect='', Status=Active.
    """
    response = client.ListRows(jql_pb2.ListRowsRequest(
        table=schema.Tables.Tasks,
        conditions=[jql_pb2.Condition(requires=[
            jql_pb2.Filter(
                column=schema.Fields.Action,
                equal_match=jql_pb2.EqualMatch(value="Plan"),
            ),
            jql_pb2.Filter(
                column=schema.Fields.Direct,
                equal_match=jql_pb2.EqualMatch(value="today"),
            ),
            jql_pb2.Filter(
                column=schema.Fields.Indirect,
                equal_match=jql_pb2.EqualMatch(value=""),
            ),
            jql_pb2.Filter(
                column=schema.Fields.Status,
                equal_match=jql_pb2.EqualMatch(value=schema.Values.StatusActive),
            ),
        ])],
    ))
    if not response.rows:
        return None
    primary, _ = list_rows_meta(response)
    return response.rows[0].entries[primary].formatted


def get_day_plan_entry_refs(client, plan_pk):
    """Returns a dict mapping reminder arg0 -> (assn_pk, order) for all
    reminders currently referenced in today's day plan."""
    if plan_pk is None:
        return {}
    response = client.ListRows(jql_pb2.ListRowsRequest(
        table=schema.Tables.Assertions,
        conditions=[jql_pb2.Condition(requires=[
            jql_pb2.Filter(
                column=schema.Fields.Arg0,
                equal_match=jql_pb2.EqualMatch(value=f"tasks {plan_pk}"),
            ),
            jql_pb2.Filter(
                column=schema.Fields.Relation,
                equal_match=jql_pb2.EqualMatch(value=".Entry"),
            ),
        ])],
    ))
    cmap = {c.name: i for i, c in enumerate(response.columns)}
    primary = next(i for i, c in enumerate(response.columns) if c.primary)
    entry_refs = {}
    for row in response.rows:
        value = row.entries[cmap[schema.Fields.Arg1]].formatted
        if is_foreign(value):
            tbl, fk_pk = parse_foreign(value)
            if tbl == "vt.reminders":
                assn_pk = row.entries[primary].formatted
                order = row.entries[cmap[schema.Fields.Order]].formatted
                entry_refs[fk_pk] = (assn_pk, order)
    return entry_refs


def find_matching_auxiliaries(client, pks, kind):
    task_pks = []
    noun_pks = []
    for fpk in pks:
        table, pk = parse_full_pk(fpk)
        if table == schema.Tables.Tasks:
            task_pks.append(pk)
        elif table == schema.Tables.Nouns:
            noun_pks.append(pk)
        else:
            raise ValueError(
                "Only tasks and nouns are supported for finding auxiliaries")
    if task_pks and noun_pks:
        raise ValueError(
            "Cannot mix tasks and nouns when finding auxiliaries")
    if task_pks:
        return _find_matching_auxiliaries_for_tasks(client, task_pks, kind)
    return _find_matching_auxiliaries_for_nouns(client, noun_pks, kind)


def _all_auxes(client, kind):
    all_auxes = client.ListRows(
        jql_pb2.ListRowsRequest(
            table=schema.Tables.Assertions,
            conditions=[
                jql_pb2.Condition(requires=[
                    jql_pb2.Filter(
                        column=schema.Fields.Relation,
                        equal_match=jql_pb2.EqualMatch(value='.Class'),
                    ),
                    jql_pb2.Filter(
                        column=schema.Fields.Arg1,
                        equal_match=jql_pb2.EqualMatch(
                            value=f'@{{nouns {kind}}}'),
                    ),
                ])
            ],
        ))
    assn_primary, assn_cmap = list_rows_meta(all_auxes)
    aux_paths = [
        aux.entries[assn_cmap[schema.Fields.Arg0]].formatted
        for aux in all_auxes.rows
    ]
    fields, _ = get_fields_for_items(client, "", aux_paths)
    return fields


def _find_matching_auxiliaries_for_nouns(client, noun_pks, kind):
    resp = client.ListRows(
        jql_pb2.ListRowsRequest(
            table=schema.Tables.Nouns,
            conditions=[
                jql_pb2.Condition(requires=[
                    jql_pb2.Filter(
                        column=schema.Fields.Identifier,
                        in_match=jql_pb2.InMatch(values=noun_pks),
                    ),
                ]),
            ],
        ))
    primary, cmap = list_rows_meta(resp)
    toret = {}
    fields = _all_auxes(client, kind)
    for row in resp.rows:
        parent = row.entries[cmap[schema.Fields.Parent]].formatted
        pk = full_pk(schema.Tables.Nouns, row.entries[primary].formatted)
        toret[pk] = []
        for aux, aux_fields in fields.items():
            _, aux_pk = parse_full_pk(aux)
            parent_matches = {f"@{{nouns {parent}}}", "*"} & set(aux_fields[f'{kind}.Parent'])
            if parent_matches:
                toret[pk].append(aux)
    return toret


def _find_matching_auxiliaries_for_tasks(client, task_pks, kind):
    resp = client.ListRows(
        jql_pb2.ListRowsRequest(
            table=schema.Tables.Tasks,
            conditions=[
                jql_pb2.Condition(requires=[
                    jql_pb2.Filter(
                        column=schema.Fields.UDescription,
                        in_match=jql_pb2.InMatch(values=task_pks),
                    ),
                ]),
            ],
        ))
    primary, cmap = list_rows_meta(resp)
    toret = {}
    fields = _all_auxes(client, kind)
    for row in resp.rows:
        action = row.entries[cmap[schema.Fields.Action]].formatted
        direct = row.entries[cmap[schema.Fields.Direct]].formatted
        pk = full_pk(schema.Tables.Tasks, row.entries[primary].formatted)
        toret[pk] = []
        for aux, aux_fields in fields.items():
            _, aux_pk = parse_full_pk(aux)
            action_matches = {action, "*"} & set(aux_fields[f'{kind}.Action'])
            direct_matches = {f"@{{nouns {direct}}}", "*"} & set(aux_fields[f'{kind}.Direct'])
            if action_matches and direct_matches:
                toret[pk].append(aux)
    return toret
