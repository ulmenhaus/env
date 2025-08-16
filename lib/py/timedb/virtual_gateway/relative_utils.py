from timedb import schema, pks
from jql import jql_pb2


def get_mapping_of_attrs(pk, pk_map):
    mapping = {
        "_pk": [pk],
    }
    for attr_name, attr_pairs in pk_map.items():
        if len(attr_pairs) == 0:
            mapping[attr_name] = [""]
        elif len(attr_pairs) == 1:
            mapping[attr_name] = [attr_pairs[0][1]]
        else:
            # Convert multiple attributes to a bulleted list so that
            # they can be edited as one text blob
            mapping[attr_name] = [
                _to_bulleted_list(value for _pk, value in attr_pairs)
            ]
    return mapping


def update_attrs(client, pk, pk_map, fields):
    for field, value in fields.items():
        new_entries = _from_bulleted_list(value)
        for assn_pk, _attr_value in pk_map.get(field, []):
            client.DeleteRow(
                jql_pb2.DeleteRowRequest(
                    table=schema.Tables.Assertions,
                    pk=assn_pk,
                ))
        for i, new_entry in enumerate(new_entries):
            fields = {
                schema.Fields.Relation: f".{field}",
                schema.Fields.Arg0: pk,
                schema.Fields.Arg1: new_entry,
                schema.Fields.Order: str(i),
            }
            client.WriteRow(
                jql_pb2.WriteRowRequest(
                    table=schema.Tables.Assertions,
                    pk=pks.pk_for_assertion(fields),
                    fields=fields,
                    insert_only=True,
                ))


def _to_bulleted_list(entries):
    return "\n".join(f"* {entry}" for entry in entries)


def _from_bulleted_list(blob):
    if not blob:
        return []
    if not blob.startswith("* "):
        return [blob]
    return blob[2:].split("\n* ")
