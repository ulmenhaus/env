import collections
import datetime
import json

from datetime import datetime, timedelta

from jql import jql_pb2
from timedb import schema

ALIAS_MODIFIER = 'alias of'


def get_fields_for_items(client, table, pks, fields=()):
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
        full_pk = f"tasks {task_pk}"
        pk2attributes[full_pk] = {
            schema.Fields.Action: action_primary,
            "Date": date,
        }
        if action_primary in actions_by_primary:
            action = actions_by_primary[action_primary]
            cls = action.entries[action_cmap[schema.Fields.Class]].formatted
            if schema.indirect_indicates_habit(indirect):
                pk2attributes[full_pk]["Class"] = "Habit"
            else:
                pk2attributes[full_pk]["Class"] = cls
            if direct:
                ps = action.entries[action_cmap[
                    schema.Fields.Direct]].formatted
                relation = schema.relation_from_parameter_schema(ps)
                pk2attributes[full_pk][relation] = f"@{{nouns {direct}}}"
            if indirect:
                ps = action.entries[action_cmap[
                    schema.Fields.Indirect]].formatted
                relation = schema.relation_from_parameter_schema(ps)
                pk2attributes[full_pk][relation] = f"@{{nouns {indirect}}}"
        else:
            if direct:
                pk2attributes[full_pk]["Direct"] = f"@{{nouns {direct}}}"
            if indirect:
                pk2attributes[full_pk]["Indirect"] = f"@{{nouns {indirect}}}"
    return pk2attributes


def apply_request_parameters(rows, request):
    if len(request.conditions) > 1:
        raise ValueError("Multiple conditions not yet supported")

    if len(request.conditions) == 1:
        filter_row = lambda row: all(
            _filter_matches(row, f) for f in request.conditions[0].requires)
        rows = list(filter(filter_row, rows))
    rows = sorted(
        rows,
        key=lambda idea: _sort_key(idea.get(request.order_by, idea["_pk"])),
        reverse=request.dec)
    return rows


def apply_request_limits(rows, request):
    limit = request.limit if request.limit else len(rows)
    return rows[request.offset:request.offset + limit]


def apply_grouping(rows, request):
    if not (request.group_by and request.group_by.groupings):
        return rows, []
    groupings = []
    for requested in request.group_by.groupings:
        field = requested.field
        selected = requested.selected
        grouping = jql_pb2.Grouping(
            field=field,
            selected=selected,
            values=dict(
                collections.Counter(
                    map(lambda row: row[field][0]
                        if row[field] else "", rows))),
        )
        rows = [row for row in rows if selected in row[field]]
        groupings.append(grouping)
    return rows, groupings


def gather_max_lens(rows, base_cols=()):
    all_keys = set(base_cols).union(*(row.keys() for row in rows))
    max_lens = {k: len(k) for k in all_keys}
    for row in rows:
        for k, v in row.items():
            max_lens[k] = max(max_lens[k], len(present_attrs(v)))
    return max_lens


def _filter_matches(row, f):
    match_type = f.WhichOneof('match')
    if match_type == 'equal_match':
        values = row[f.column] if row[f.column] else [""]
        return (f.equal_match.value in values) ^ f.negated
    elif match_type == 'contains_match':
        return (f.contains_match.value.lower() in "\n".join(
            row.get(f.column, "")).lower()) ^ f.negated
    else:
        raise ValueError("Unknown filter type", match_type)


def link_attrs(attrs):
    if len(attrs) != 1:
        return ""
    attr, = attrs
    return strip_foreign(attr) if is_foreign(attr) else ""


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
                if table == "ratings":
                    num, denom = map(int, pk.split(" "))
                    return "●" * num + "○" * (denom - num)
                return pk
            except:
                pass
        return entry
    return f"{len(attrs)} entries"


def _sort_key(attrs):
    as_shown = present_attrs(attrs)
    try:
        # HACK use a highly padded string as the sort key for an entry that
        # begins with a number so we can mix numerical and lexicographic sorting
        return str(int(as_shown.split(" ")[0].split("%")[0].replace(
            ",", ""))).zfill(40)
    except ValueError:
        return as_shown


def get_primary(response):
    return [i for i, c in enumerate(response.columns) if c.primary][0]


def encode_pk(noun_pk, assn_pks):
    return "\t".join([noun_pk, json.dumps(assn_pks)])


def decode_pk(pk):
    noun_pk, assn_pks = pk.split("\t")
    return noun_pk, json.loads(assn_pks)


def is_encoded_pk(pk):
    return "\t" in pk


def possible_targets(client, request, table):
    tgt_tables = [schema.Tables.Nouns, schema.Tables.Tasks]
    entries = []
    for tgt_table in tgt_tables:
        tgt_request = jql_pb2.ListRowsRequest(table=tgt_table)
        response = client.ListRows(tgt_request)
        primary, = [i for i, c in enumerate(response.columns) if c.primary]
        pks = [
            f"{tgt_table} {row.entries[primary].formatted}"
            for row in response.rows
        ]
        entries.extend({"_pk": [pk], "-> Item": [pk]} for pk in pks)
    filtered = apply_request_parameters(entries, request)
    final = apply_request_limits(filtered, request)
    return jql_pb2.ListRowsResponse(
        table=table,
        columns=[jql_pb2.Column(name="-> Item", max_length=30, primary=True)],
        rows=[
            jql_pb2.Row(entries=[jql_pb2.Entry(formatted=entry["-> Item"][0])])
            for entry in final
        ],
        total=len(filtered),
        all=len(entries),
    )


def selected_target(request):
    for condition in request.conditions:
        for f in condition.requires:
            match_type = f.WhichOneof('match')
            if match_type == "equal_match" and f.column == '-> Item':
                return f.equal_match.value


def is_foreign(entry):
    # TODO technically we should look for colons in the middle, but some pks right now
    # have colons in the middle that should be escaped first
    if not entry:
        return True
    return entry.startswith("@{") and entry.endswith("}")


def parse_full_pk(full_pk):
    return full_pk.split(" ", 1)


def parse_foreign(entry):
    return parse_full_pk(strip_foreign(entry))


def full_pk(table, pk):
    return f"{table} {pk}"


def strip_foreign(entry):
    return entry[len("@{"):-len("}")]


def strip_foreign_noun(entry):
    return entry[len("@{nouns "):-1]


def _mapping_to_row(mapping, fields):
    return jql_pb2.Row(entries=[
        jql_pb2.Entry(
            formatted=format_attrs(mapping.get(field, [])),
            display_value=present_attrs(mapping.get(field, [])),
            link=link_attrs(mapping.get(field, [])),
        ) for field in fields
    ])


def _fields_to_columns(fields, type_of=None, values=None, max_lens=None):
    type_of = type_of or {}
    values = values or {}
    max_lens = max_lens or {}
    return [
        jql_pb2.Column(
            name=field,
            type=type_of.get(field, jql_pb2.EntryType.STRING),
            values=values.get(field, []),
            max_length=max_lens.get(field, 10),
            # TODO we probably don't need each caller to provide a _pk field
            # and instead can use the key in the provieded dict as _pk
            primary=field == '_pk') for field in fields
    ]


def list_rows(table_name, rows, request, values=None, allow_foreign=True):
    values = values if values else {}
    type_of = {k: jql_pb2.EntryType.ENUM for k in values}
    filtered = apply_request_parameters(rows.values(), request)
    grouped, groupings = apply_grouping(filtered, request)
    max_lens = gather_max_lens(grouped, [])
    final = apply_request_limits(grouped, request)
    fields = sorted(set().union(*(final)) - {"-> Item"}) or ["None"]
    return jql_pb2.ListRowsResponse(
        table=table_name,
        columns=_fields_to_columns(fields, type_of, values, max_lens),
        rows=[_mapping_to_row(relative, fields) for relative in final],
        all=len(rows),
        total=len(grouped),
        groupings=groupings,
    )


def return_row(table_name, row):
    fields = sorted(set(row.keys()) - {"-> Item"}) or ["None"]
    return jql_pb2.GetRowResponse(
        table=table_name,
        columns=_fields_to_columns(fields),
        row=jql_pb2.Row(entries=[
            jql_pb2.Entry(
                formatted=format_attrs(row.get(field, [])),
                display_value=present_attrs(row.get(field, [])),
                link=link_attrs(row.get(field, [])),
            ) for field in fields
        ]),
    )


# TODO replace every in-line construct of primary and cmap with this helper
def list_rows_meta(resp):
    primary, = [i for i, c in enumerate(resp.columns) if c.primary]
    cmap = {c.name: i for i, c in enumerate(resp.columns)}
    return primary, cmap


class TimingInfo(object):

    def __init__(self, days_since, days_until, cadence, cadence_pk, active_actions):
        self.days_since = days_since
        self.days_until = days_until
        self.cadence = cadence
        self.cadence_pk = cadence_pk
        self.active_actions = active_actions


def get_timing_info(client, noun_pks):
    info = {}
    fields, assn_pks = get_fields_for_items(client, schema.Tables.Nouns,
                                            noun_pks,
                                            ["Cadence", "StartDate", "Cost"])
    all_days_since = _days_since(client, noun_pks)
    for noun_pk in noun_pks:
        cadence = fields[noun_pk].get("Cadence", [""])[0]
        start_date = fields[noun_pk].get("StartDate", [""])[0]
        cost = fields[noun_pk].get("Cost", [""])[0]
        # The item is still active in the pipeline so we go based off
        # of its start date
        if start_date and cost:
            parsed_start = datetime.strptime(start_date, '%Y-%m-%d')
            expected_days = 2 * (3**(int(cost.split(" ")[0]) - 1))
            expected_end = parsed_start + timedelta(days=expected_days)
            days_until_int = (expected_end - datetime.now()).days
            if days_until_int > 0:
                days_until = "+" + str(days_until_int).zfill(4)
            else:
                days_until = str(days_until_int).zfill(5)
            info[noun_pk] = TimingInfo(
                str((datetime.now() - parsed_start).days).zfill(4), days_until,
                "", "", set())
        # If a cadence is set for the noun then it is habitual and we just calculate
        # based on the cadence and the last time the task was done
        else:
            if noun_pk in all_days_since:
                days_since_int, active_actions = all_days_since[noun_pk]
                days_since = str(days_since_int).zfill(4)
            else:
                days_since, active_actions = "", set()
            days_until = ""
            if cadence and days_since:
                cadence_period = int(cadence.split(" ")[0])
                days_until_int = cadence_period - int(days_since)
                if days_until_int > 0:
                    days_until = "+" + str(days_until_int).zfill(4)
                else:
                    days_until = str(days_until_int).zfill(5)
            info[noun_pk] = TimingInfo(
                days_since,
                days_until,
                cadence,
                assn_pks.get(noun_pk, ""),
                active_actions,
            )
    return info


def _days_since(client, noun_pks):
    noun_to_tasks = collections.defaultdict(list)
    ret = {}
    # TODO once we support multiple conditions this can be done
    # in one query. Other nicities that would make this UX nicer
    #
    # 1. Group by direct/indirect and limit 1 so we get the exact
    #    entry we want
    # 2. Provide a format string for the date entry so we can
    #    convert to UNIX timestamp on the server side
    # 3. Filter columns to just the relevant ones

    # Get matching tasks based on direct/indirect
    for column in [schema.Fields.Direct, schema.Fields.Indirect]:
        tasks_request = jql_pb2.ListRowsRequest(
            table=schema.Tables.Tasks,
            conditions=[
                jql_pb2.Condition(requires=[
                    jql_pb2.Filter(
                        column=column,
                        in_match=jql_pb2.InMatch(values=noun_pks),
                    ),
                ]),
            ],
        )
        tasks_response = client.ListRows(tasks_request)
        _, tasks_cmap = list_rows_meta(tasks_response)
        for row in tasks_response.rows:
            noun_to_tasks[row.entries[tasks_cmap[column]].formatted].append(
                row)

    # Get matching tasks based on properties
    references = [f"@{{nouns {noun_pk}}}" for noun_pk in noun_pks]
    assn_request = jql_pb2.ListRowsRequest(
        table=schema.Tables.Assertions,
        conditions=[
            jql_pb2.Condition(requires=[
                jql_pb2.Filter(
                    column=schema.Fields.Arg1,
                    in_match=jql_pb2.InMatch(values=references),
                ),
            ]),
        ],
    )
    assn_response = client.ListRows(assn_request)
    _, assn_cmap = list_rows_meta(assn_response)
    task_pk_to_nouns = collections.defaultdict(set)
    for row in assn_response.rows:
        noun = strip_foreign_noun(
            row.entries[assn_cmap[schema.Fields.Arg1]].formatted)
        table, pk = row.entries[assn_cmap[schema.Fields.Arg0]].formatted.split(
            " ", 1)
        if table != schema.Tables.Tasks:
            continue
        task_pk_to_nouns[pk].add(noun)

    tasks_request = jql_pb2.ListRowsRequest(
        table=schema.Tables.Tasks,
        conditions=[
            jql_pb2.Condition(requires=[
                jql_pb2.Filter(
                    column=schema.Fields.UDescription,
                    in_match=jql_pb2.InMatch(values=list(task_pk_to_nouns)),
                ),
            ]),
        ],
    )
    tasks_response = client.ListRows(tasks_request)
    tasks_primary, tasks_cmap = list_rows_meta(tasks_response)
    for task in tasks_response.rows:
        pk = task.entries[tasks_primary].formatted
        for noun in task_pk_to_nouns[pk]:
            noun_to_tasks[noun].append(task)
    # Finally iterate over all matching tasks for nouns and construct out response
    for noun, tasks in noun_to_tasks.items():
        for task in tasks:
            start_formatted = task.entries[tasks_cmap[
                schema.Fields.ParamStart]].formatted
            days_since = (datetime.now() -
                          datetime.strptime(start_formatted, "%d %b %Y")).days
            action = task.entries[tasks_cmap[schema.Fields.Action]].formatted
            indirect = task.entries[tasks_cmap[schema.Fields.Indirect]].formatted
            status = task.entries[tasks_cmap[schema.Fields.Status]].formatted

            active_actions = ret[noun][1] if noun in ret else set()
            if status in schema.active_statuses():
                active_actions.add((action, indirect))
            if (noun not in ret) or (ret[noun][0] > days_since):
                final_days_since = days_since
            else:
                final_days_since = ret[noun][0]
            ret[noun] = final_days_since, active_actions
    return ret


def get_row(list_resp, pk):
    primary = get_primary(list_resp)
    for row in list_resp.rows:
        if row.entries[primary].formatted == pk:
            return jql_pb2.GetRowResponse(
                table='vt.practices',
                columns=list_resp.columns,
                row=row,
            )
    raise ValueError("no such pk", pk)
