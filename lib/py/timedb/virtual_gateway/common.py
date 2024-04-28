import collections
import json

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
    all_count = len(rows)
    limit = request.limit if request.limit else all_count
    rows = rows[request.offset:request.offset + limit]
    return rows, all_count


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
        return str(int(as_shown.split(" ")[0].replace(",", ""))).zfill(40)
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
    final, all_count = apply_request_parameters(entries, request)
    return jql_pb2.ListRowsResponse(
        table=table,
        columns=[jql_pb2.Column(name="-> Item", max_length=30, primary=True)],
        rows=[
            jql_pb2.Row(entries=[jql_pb2.Entry(formatted=noun["-> Item"][0])])
            for noun in final
        ],
        total=all_count,
        all=len(entries),
    )


def selected_target(request):
    for condition in request.conditions:
        for f in condition.requires:
            match_type = f.WhichOneof('match')
            if match_type == "equal_match" and f.column == '-> Item':
                return f.equal_match.value


def _is_foreign(entry):
    return len(entry) > len("@timedb:") and entry.startswith(
        "@timedb:") and entry.endswith(":") and ":" not in _strip_foreign(
            entry)


def foreign_fields(rows):
    all_fields = set()
    not_foreign = set()
    for row in rows:
        for k, v in row.items():
            all_fields.add(k)
            for item in v:
                if not _is_foreign(item):
                    not_foreign.add(k)
    return all_fields - not_foreign


def _strip_foreign(entry):
    return entry[len("@timedb:"):-1]


def convert_foreign_fields(before, foreign):
    after = []
    for row in before:
        new_row = collections.defaultdict(list)
        for k, v in row.items():
            if k in foreign:
                # For now we only allow referencing nouns from assertions, but we may support other tables in the future
                new_row[k] = [f"nouns {_strip_foreign(item)}" for item in v]
            else:
                new_row[k] = v
        after.append(new_row)
    return after
