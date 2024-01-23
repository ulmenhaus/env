import collections

from jql import jql_pb2
from timedb import schema


def get_fields_for_items(client, table, pks, fields):
    ret = {pk: {field: "" for field in fields} for pk in pks}
    assertions_request = jql_pb2.ListRowsRequest(
        table=schema.Tables.Assertions,
        conditions=[
            jql_pb2.Condition(requires=[
                jql_pb2.Filter(
                    column=schema.Fields.Arg0,
                    in_match=jql_pb2.InMatch(
                        values=[f"{table} {pk}" for pk in pks]),
                ),
                jql_pb2.Filter(
                    column=schema.Fields.Relation,
                    in_match=jql_pb2.InMatch(
                        values=[f".{field}" for field in fields]),
                ),
            ]),
        ],
    )
    assertions_response = client.ListRows(assertions_request)
    assn_cmap = {c.name: i for i, c in enumerate(assertions_response.columns)}
    assn_primary, = [i for i, c in enumerate(assertions_response.columns) if c.primary]
    assn_pks = collections.defaultdict(dict)
    for row in assertions_response.rows:
        rel = row.entries[assn_cmap[schema.Fields.Relation]].formatted
        value = row.entries[assn_cmap[schema.Fields.Arg1]].formatted
        arg0 = row.entries[assn_cmap[schema.Fields.Arg0]].formatted
        pk = arg0.split(" ", 1)[1]
        ret[pk][rel[1:]] = value
        assn_pks[pk][rel[1:]] = [row.entries[assn_primary].formatted, value]
    return ret, assn_pks

def apply_request_parameters(rows, request):
    if len(request.conditions) > 1:
        raise ValueError("Multiple conditions not yet supported")

    if len(request.conditions) == 1:
        filter_row = lambda row: all(_filter_matches(row, f) for f in request.conditions[0].requires)
        rows = list(filter(filter_row, rows))
    rows = sorted(rows, key=lambda idea: idea.get(request.order_by, idea["_pk"]), reverse=request.dec)
    all_count = len(rows)
    rows = rows[request.offset:request.offset + request.limit]
    return rows, all_count

def _filter_matches(row, f):
    match_type = f.WhichOneof('match')
    if match_type == 'equal_match':
        return (row[f.column] == f.equal_match.value) ^ f.negated
    elif match_type == 'contains_match':
        return (f.contains_match.value.lower() in row[f.column].lower()) ^ f.negated
    else:
        raise ValueError("Unknown filter type", match_type)
