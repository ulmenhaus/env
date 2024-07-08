import collections
import json

from datetime import datetime, timedelta

from jql import jql_pb2
from timedb import schema

ALIAS_MODIFIER = 'alias of'


def get_fields_for_items(client, table, pks, fields=()):
    prefix = f"{table} " if table else ""
    ret = {pk: collections.defaultdict(list) for pk in pks}
    filters = [
        jql_pb2.Filter(
            column=schema.Fields.Arg0,
            in_match=jql_pb2.InMatch(values=[f"{prefix}{pk}" for pk in pks]),
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
    assn_pks = collections.defaultdict(dict)
    for row in assertions_response.rows:
        rel = row.entries[assn_cmap[schema.Fields.Relation]].formatted
        value = row.entries[assn_cmap[schema.Fields.Arg1]].formatted
        arg0 = row.entries[assn_cmap[schema.Fields.Arg0]].formatted
        pk = arg0.split(" ", 1)[1] if table else arg0
        ret[pk][rel[1:]].append(value)
        assn_pks[pk][rel[1:]] = [row.entries[assn_primary].formatted, value]
    return ret, assn_pks


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
            row[f.column]).lower()) ^ f.negated
    else:
        raise ValueError("Unknown filter type", match_type)


def present_attrs(attrs):
    if len(attrs) == 0:
        return ""
    if len(attrs) == 1:
        if attrs[0].startswith("@timedb:") and attrs[0].endswith(":"):
            inner = attrs[0][len("@timedb:"):-1]
            if inner and ":" not in inner:
                # Disabling this behavior for now
                # return inner
                pass
        return attrs[0]
    return f"{len(attrs)} entries"


def _sort_key(attrs):
    as_shown = present_attrs(attrs)
    try:
        # HACK use a highly padded string as the sort key for an entry that
        # begins with a number so we can mix numerical and lexicographic sorting
        return str(int(as_shown.split(" ")[0].split("%")[0].replace(",", ""))).zfill(40)
    except ValueError:
        return as_shown


def get_primary(response):
    return [i for i, c in enumerate(response.columns) if c.primary][0]


def encode_pk(noun_pk, assn_pks):
    return "\t".join([noun_pk, json.dumps(assn_pks)])


def decode_pk(pk):
    noun_pk, assn_pks = pk.split("\t")
    return noun_pk, json.loads(assn_pks)


def possible_targets(client, request, table):
    nouns_request = jql_pb2.ListRowsRequest(table=schema.Tables.Nouns)
    nouns_response = client.ListRows(nouns_request)
    primary, = [i for i, c in enumerate(nouns_response.columns) if c.primary]
    nouns_cmap = {c.name: i for i, c in enumerate(nouns_response.columns)}
    noun_pks = [
        f"{row.entries[primary].formatted}" for row in nouns_response.rows
    ]
    entries = [{
        "_pk": [pk],
        "-> Item": [f"{schema.Tables.Nouns} {pk}"]
    } for pk in noun_pks]
    final = apply_request_parameters(entries, request)
    return jql_pb2.ListRowsResponse(
        table=table,
        columns=[jql_pb2.Column(name="-> Item", max_length=30, primary=True)],
        rows=[
            jql_pb2.Row(entries=[jql_pb2.Entry(formatted=noun["-> Item"][0])])
            for noun in final
        ],
        total=len(entries),
        all=len(noun_pks),
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
    if entry.startswith("@{") and entry.endswith("}"):
        return True
    return len(entry) > len("@timedb:") and entry.startswith(
        "@timedb:") and entry.endswith(":")


def foreign_fields(rows):
    field_to_tables = collections.defaultdict(set)
    fields_with_non_foreigns = set()
    for row in rows:
        for k, v in row.items():
            for item in v:
                # Eventually we will support entries like @:nouns <pk>" and @:tasks <pk>:
                # but for now the only foreign key references are nouns referenced
                # as @timedb:<pk>:
                field_to_tables[k].add("nouns")
                if not is_foreign(item):
                    fields_with_non_foreigns.add(k)
    return {
        k: v
        for k, v in field_to_tables.items()
        if k not in fields_with_non_foreigns
    }


def parse_foreign(entry):
    polyforeign = entry[len("@{"):-len("}")]
    return polyforeign.split(" ", 1)

def strip_foreign(entry):
    return entry[len("@timedb:"):-1]


def convert_foreign_fields(before, foreign):
    after = []
    for row in before:
        new_row = collections.defaultdict(list)
        for k, v in row.items():
            if k in foreign:
                # For now we only allow referencing nouns from assertions, but we may support other tables in the future
                new_row[k] = list(map(strip_foreign, v))
            else:
                new_row[k] = v
        after.append(new_row)
    return after


def list_rows(table_name, rows, request, values=None, allow_foreign=True):
    values = values if values else {}
    type_of = {k: jql_pb2.EntryType.ENUM for k in values}
    field_to_tables = foreign_fields(rows.values()) if allow_foreign else {}
    foreign_tables = {}
    for field, tables in field_to_tables.items():
        type_of[field] = jql_pb2.EntryType.FOREIGN if len(
            tables) == 1 else jql_pb2.EntryType.POLYFOREIGN
        if len(tables) == 1:
            foreign_tables[field] = list(tables)[0]
    converted = convert_foreign_fields(rows.values(), field_to_tables)
    filtered = apply_request_parameters(converted, request)
    grouped, groupings = apply_grouping(filtered, request)
    max_lens = gather_max_lens(grouped, [])
    final = apply_request_limits(grouped, request)
    fields = sorted(set().union(*(final)) - {"-> Item"}) or ["None"]
    return jql_pb2.ListRowsResponse(
        table=table_name,
        columns=[
            jql_pb2.Column(
                name=field,
                type=type_of.get(field, jql_pb2.EntryType.STRING),
                foreign_table=foreign_tables.get(field, ''),
                values=values.get(field, []),
                max_length=max_lens.get(field, 10),
                # TODO we probably don't need each caller to provide a _pk field
                # and instead can use the key in the provieded dict as _pk
                primary=field == '_pk') for field in fields
        ],
        rows=[
            jql_pb2.Row(entries=[
                jql_pb2.Entry(formatted=present_attrs(relative[field]))
                for field in fields
            ]) for relative in final
        ],
        all=len(converted),
        total=len(grouped),
        groupings=groupings,
    )


# TODO replace every in-line construct of primary and cmap with this helper
def list_rows_meta(resp):
    primary, = [i for i, c in enumerate(resp.columns) if c.primary]
    cmap = {c.name: i for i, c in enumerate(resp.columns)}
    return primary, cmap


class TimingInfo(object):

    def __init__(self, days_since, days_until, cadence, cadence_pk):
        self.days_since = days_since
        self.days_until = days_until
        self.cadence = cadence
        self.cadence_pk = cadence_pk


def get_timing_info(client, noun_pks):
    info = {}
    fields, assn_pks = get_fields_for_items(client, schema.Tables.Nouns,
                                            noun_pks, ["Cadence", "StartDate", "Cost"])
    all_days_since = _days_since(client, noun_pks)
    for noun_pk in noun_pks:
        cadence = fields[noun_pk].get("Cadence", [""])[0]
        start_date = fields[noun_pk].get("StartDate", [""])[0]
        cost = fields[noun_pk].get("Cost", [""])[0]
        # If a cadence is set for the noun then it is habitual and we just calculate
        # based on the cadence and the last time the task was done
        if cadence:
            days_since = str(all_days_since[noun_pk]).zfill(4) if noun_pk in all_days_since else ""
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
            )
        # Otherwise the item is still active in the pipeline so we go based off
        # of its start date
        elif start_date and cost:
            parsed_start = datetime.strptime(start_date, '%Y-%m-%d')
            expected_days = 2 * (3 ** (int(cost.split(" ")[0]) - 1))
            expected_end = parsed_start + timedelta(days=expected_days)
            days_until_int = (expected_end - datetime.now()).days
            if days_until_int > 0:
                days_until = "+" + str(days_until_int).zfill(4)
            else:
                days_until = str(days_until_int).zfill(5)
            info[noun_pk] = TimingInfo(
                str((datetime.now() - parsed_start).days).zfill(4),
                days_until,
                "",
                ""
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
            noun_to_tasks[row.entries[tasks_cmap[column]].formatted].append(row)

    # Get matching tasks based on properties
    references = [f"@timedb:{noun_pk}:" for noun_pk in noun_pks]
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
        noun = strip_foreign(row.entries[assn_cmap[schema.Fields.Arg1]].formatted)
        table, pk = row.entries[assn_cmap[schema.Fields.Arg0]].formatted.split(" ", 1)
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
            if (noun not in ret) or (ret[noun] > days_since):
                ret[noun] = days_since
    return ret

def get_row(list_resp, pk):
    primary = get_primary(list_resp)
    for row in list_resp.rows:
        if row.entries[primary].formatted == pk:
            return jql_pb2.GetRowResponse(
                table='vt.practices',
                columns = list_resp.columns,
                row=row,
            )
    raise ValueError("no such pk", pk)
