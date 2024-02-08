import collections

from jql import jql_pb2
from timedb import schema


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
    rows = sorted(rows,
                  key=lambda idea: _sort_key(
                      idea.get(request.order_by, idea["_pk"])),
                  reverse=request.dec)
    all_count = len(rows)
    rows = rows[request.offset:request.offset + request.limit]
    return rows, all_count


def gather_max_lens(rows):
    all_keys = set().union(*(row.keys() for row in rows))
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
                return inner
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
 
