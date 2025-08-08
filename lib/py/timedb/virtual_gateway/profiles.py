import collections
import json

from timedb import pks, schema
from timedb.virtual_gateway import common

from jql import jql_pb2, jql_pb2_grpc


class ProfilesBackend(jql_pb2_grpc.JQLServicer):

    def __init__(self, client):
        super().__init__()
        self.client = client

    def ListRows(self, request, context):
        targets = common.selected_targets(request, column='_target')
        if targets is None:
            # The first call from the client just gets meta-data
            return common.list_rows('vt.profiles', {}, request)
        profiles = common.find_matching_auxiliaries(self.client, targets,
                                                    "Schema")
        distinct_profiles = list(set().union(*profiles.values()))
        profile_fields, _ = common.get_fields_for_items(self.client,
                                                        "",
                                                        distinct_profiles,
                                                        fields=['Dimension'])
        target_fields, target_assn_pks = common.get_fields_for_items(
            self.client, "", targets)
        rows = {}
        schema_values = self._get_values_for_schemas(profile_fields)
        invariant_attrs = ["A Date"]
        for target, matching_profiles in profiles.items():
            for profile in matching_profiles:
                attrs = {
                    "_target": target,
                    "pk": [f"@{{{target}}}"],
                    "Profile": [profile],
                }
                for field in profile_fields[profile]['Dimension']:
                    attrs[field] = []
                for field, values in target_fields[target].items():
                    if field in profile_fields[profile][
                            'Dimension'] or field in invariant_attrs:
                        attrs[field] = values
                row_id = _encode_row_id(target, profile)
                pk = common.encode_pk(row_id, target_assn_pks[target])
                attrs["_pk"] = [pk]
                rows[pk] = attrs
        return common.list_rows(
            'vt.profiles',
            rows,
            request,
            values=schema_values,
            client=self.client,
            hide_grouping_fields=True,
        )

    def _get_values_for_schemas(self, fields):
        distinct_schemas = list(set().union(*(field['Dimension']
                                              for field in fields.values())))
        schema_full_pks = [
            common.strip_foreign(ds) for ds in distinct_schemas
            if common.is_foreign(ds)
            and common.parse_foreign(ds)[0] == schema.Tables.Nouns
        ]
        schema_attrs, _ = common.get_fields_for_items(self.client,
                                                      "",
                                                      schema_full_pks,
                                                      fields=['ValueSet'])
        schema2values = {}
        for schema_name, fields in schema_attrs.items():
            value_set = fields['ValueSet']
            schema_key = f"@{{{schema_name}}}"
            schema2values[schema_key] = []
            for value_def in value_set:
                # NOTE if we have multiple @{nouns } style defs then getting them serially
                # is a little inefficient, but is worth the tradeoff for cleaner code
                schema2values[schema_key].extend(
                    self._get_schema_values(value_def))
        return schema2values

    def _get_schema_values(self, schema_value_def):
        table, pk = common.parse_foreign(schema_value_def)
        if table == "ratings":
            total = int(pk)
            return [f"@{{ratings {i} {total}}}" for i in range(total + 1)]
        if table == "ints":
            return []
        if table == schema.Tables.Nouns:
            # NOTE For now we only get direct children. We can revisit this decision
            # as we home in on what exactly a taxonomy looks like.
            nouns_request = jql_pb2.ListRowsRequest(
                table=schema.Tables.Nouns,
                conditions=[
                    jql_pb2.Condition(requires=[
                        jql_pb2.Filter(
                            column=schema.Fields.Parent,
                            equal_match=jql_pb2.EqualMatch(value=pk),
                        ),
                    ], )
                ],
            )
            nouns_response = self.client.ListRows(nouns_request)
            primary, _ = common.list_rows_meta(nouns_response)
            noun_pks = [
                noun.entries[primary].formatted for noun in nouns_response.rows
            ]
            return [
                f"@{{{schema.Tables.Nouns} {noun_pk}}}" for noun_pk in noun_pks
            ]

    def WriteRow(self, request, context):
        row_id, pk_map = common.decode_pk(request.pk)
        target, _ = _decode_row_id(row_id)
        for field, value in request.fields.items():
            assn_pks = pk_map.get(field, [])
            if len(assn_pks) == 0:
                fields = {
                    schema.Fields.Relation: f".{field}",
                    schema.Fields.Arg0: target,
                    schema.Fields.Arg1: value,
                    schema.Fields.Order: "0",
                }
                self.client.WriteRow(
                    jql_pb2.WriteRowRequest(
                        table=schema.Tables.Assertions,
                        pk=pks.pk_for_assertion(fields),
                        fields=fields,
                        insert_only=True,
                    ))
            elif len(assn_pks) == 1:
                pk_pair, = assn_pks
                assn_pk, _ = pk_pair
                self.client.WriteRow(
                    jql_pb2.WriteRowRequest(
                        table=schema.Tables.Assertions,
                        pk=assn_pk,
                        fields={schema.Fields.Arg1: value},
                        update_only=True,
                    ))
            else:
                raise ValueError(
                    "Can only modify one attribute from the profiles table")
        return jql_pb2.WriteRowResponse()


def _encode_row_id(target, profile):
    return json.dumps([target, profile])


def _decode_row_id(encoded):
    return json.loads(encoded)
