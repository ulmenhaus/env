import collections
import datetime
import json

from datetime import datetime, timedelta

from jql import jql_pb2
from timedb import schema
from timedb.client_utils import (
    format_attrs,
    get_fields_for_items,
    get_primary,
    is_foreign,
    link_attrs,
    list_rows_meta,
    parse_foreign,
    parse_full_pk,
    present_attrs,
    strip_foreign_noun,
)

ALIAS_MODIFIER = 'alias of'

STATUS_VALUES = ["Awaiting", "Ready", "Done", "Elided", "Failed"]

_STATUS_COLORS = {
    "Awaiting": "\033[33m",
    "Ready":    "\033[34m",
    "Done":     "\033[32m",
    "Elided":   "\033[31m",
    "Failed":   "\033[31m",
}
_ANSI_RESET = "\033[0m"


def colorize_status(status):
    color = _STATUS_COLORS.get(status, "")
    return f"{color}{status}{_ANSI_RESET}" if color else status


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
    # Select a default value for each grouping if none is provided
    field_to_selected = {}
    for requested in request.group_by.groupings:
        field = requested.field
        selected = requested.selected
        if selected:
            field_to_selected[field] = selected
            continue
        for row in rows:
            if row[field]:
                field_to_selected[field] = row[field][0]
                break

    # Break up rows by groupings
    groupings = []
    for requested in request.group_by.groupings:
        field = requested.field
        selected = field_to_selected[field]
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
    max_lens = {k: 0 for k in all_keys}
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
    elif match_type == 'in_match':
        values = row[f.column] if row[f.column] else [""]
        return bool(set(f.in_match.values) & set(values))
    else:
        raise ValueError("Unknown filter type", match_type)


def _sort_key(attrs):
    as_shown = present_attrs(attrs)
    try:
        # HACK use a highly padded string as the sort key for an entry that
        # begins with a number so we can mix numerical and lexicographic sorting
        return str(int(as_shown.split(" ")[0].split("%")[0].replace(
            ",", ""))).zfill(40)
    except ValueError:
        return as_shown


def _mapping_to_row(mapping, fields, display_overrides=None):
    display_overrides = display_overrides or {}
    entries = []
    for field in fields:
        attrs = mapping.get(field, [])
        formatted = format_attrs(attrs)
        display_value = present_attrs(attrs)
        if field in display_overrides and formatted:
            display_value = display_overrides[field](formatted)
        entries.append(jql_pb2.Entry(
            formatted=formatted,
            display_value=display_value,
            link=link_attrs(attrs),
        ))
    return jql_pb2.Row(entries=entries)


def _fields_to_columns(fields,
                       type_of=None,
                       values=None,
                       max_lens=None,
                       client=None):
    type_of = type_of or {}
    values = values or {}
    max_lens = max_lens or {}
    display_values = collections.defaultdict(str)
    foreign_ref_columns = {
        parse_foreign(field)[1]: field
        for field in fields
        if is_foreign(field) and parse_foreign(field)[0] == schema.Tables.Nouns
    }
    if client is not None and foreign_ref_columns:
        nouns_request = jql_pb2.ListRowsRequest(
            table=schema.Tables.Nouns,
            conditions=[
                jql_pb2.Condition(requires=[
                    jql_pb2.Filter(
                        column=schema.Fields.Identifier,
                        in_match=jql_pb2.InMatch(
                            values=list(foreign_ref_columns.keys())),
                    ),
                ], )
            ],
        )
        nouns_response = client.ListRows(nouns_request)
        primary, cmap = list_rows_meta(nouns_response)
        for row in nouns_response.rows:
            row_primary = row.entries[primary].formatted
            row_short = row.entries[cmap[schema.Fields.Description]].formatted
            display_values[foreign_ref_columns[row_primary]] = row_short
    local_max_lens = dict(max_lens)
    for field in fields:
        display_value = display_values[field] or field
        if len(display_value) > local_max_lens.get(field, 10):
            local_max_lens[field] = len(display_value)
    return [
        jql_pb2.Column(
            name=field,
            type=type_of.get(field, jql_pb2.EntryType.STRING),
            values=values.get(field, []),
            max_length=local_max_lens.get(field, 10),
            display_value=display_values[field],
            # TODO we probably don't need each caller to provide a _pk field
            # and instead can use the key in the provieded dict as _pk
            primary=field == '_pk') for field in fields
    ]


def list_rows(
    table_name,
    rows,
    request,
    values=None,
    allow_foreign=True,
    client=None,
    hide_grouping_fields=False,
    display_overrides=None,
):
    values = values if values else {}
    type_of = {k: jql_pb2.EntryType.ENUM for k in values}
    filtered = apply_request_parameters(rows.values(), request)
    grouped, groupings = apply_grouping(filtered, request)
    max_lens = gather_max_lens(grouped, [])
    final = apply_request_limits(grouped, request)
    if hide_grouping_fields:
        for row in final:
            for grouping in groupings:
                if grouping.field in row:
                    del row[grouping.field]
    fields = sorted(set().union(*(final)) - {"-> Item"}) or ["None"]
    return jql_pb2.ListRowsResponse(
        table=table_name,
        columns=_fields_to_columns(fields,
                                   type_of,
                                   values,
                                   max_lens,
                                   client=client),
        rows=[_mapping_to_row(relative, fields, display_overrides) for relative in final],
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


class TimingInfo(object):

    def __init__(self, days_since, days_until, cadence, cadence_pk,
                 active_actions):
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
            indirect = task.entries[tasks_cmap[
                schema.Fields.Indirect]].formatted
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
