import collections
import json

from timedb import pks, schema
from timedb.virtual_gateway import common
from timedb import client_utils
from timedb.virtual_gateway import relative_utils, schema_values

from jql import jql_pb2, jql_pb2_grpc


class ProfilesBackend(jql_pb2_grpc.JQLServicer):

    def __init__(self, client):
        super().__init__()
        self.client = client

    def ListRows(self, request, context):
        targets = client_utils.selected_targets(request, column='_target')
        if targets is None:
            # The first call from the client just gets meta-data
            return common.list_rows('vt.profiles', {}, request)
        profiles = client_utils.find_matching_auxiliaries(self.client, targets,
                                                    "Schema")
        distinct_profiles = list(set().union(*profiles.values()))
        profile_to_dimensions = schema_values.get_dimensions(
            self.client, distinct_profiles)
        target_fields, target_assn_pks = client_utils.get_fields_for_items(
            self.client, "", targets)
        rows = {}
        all_profile_fields = set().union(*profile_to_dimensions.values())
        schema2values = schema_values.get_values_for_schemas(
            self.client, all_profile_fields)
        invariant_attrs = ["A Date"]
        all_fields = set().union(*(target_fields.values()))
        all_unprofiled_fields = all_fields - all_profile_fields
        profile_to_dimensions["unprofiled"] = list(all_unprofiled_fields)
        for target, matching_profiles in profiles.items():
            if set(target_fields[target].keys()) & all_unprofiled_fields:
                matching_profiles = matching_profiles + ["unprofiled"]
            for profile, dimensions in profile_to_dimensions.items():
                if profile not in matching_profiles:
                    continue
                attrs = {
                    "_target": [target],
                    "Profile": [profile],
                }
                for field in dimensions:
                    attrs[field] = []
                for field, values in target_fields[target].items():
                    if field in dimensions or field in invariant_attrs:
                        attrs[field] = values
                row_id = _encode_row_id(target, profile)
                pk = client_utils.encode_pk(row_id, target_assn_pks[target])
                attrs["_pk"] = [pk]
                rows[pk] = attrs
        return common.list_rows(
            'vt.profiles',
            rows,
            request,
            values=schema2values,
            client=self.client,
            hide_grouping_fields=True,
        )

    def GetRow(self, request, context):
        row_id, assn_pks = client_utils.decode_pk(request.pk)
        target, profile = _decode_row_id(row_id)
        pk, pk_map = client_utils.decode_pk(request.pk)
        mapping = relative_utils.get_mapping_of_attrs(target, assn_pks)
        return common.return_row('vt.profiles', mapping)

    def WriteRow(self, request, context):
        row_id, pk_map = client_utils.decode_pk(request.pk)
        target, _ = _decode_row_id(row_id)
        relative_utils.update_attrs(self.client, target, pk_map,
                                    request.fields)
        return jql_pb2.WriteRowResponse()


def _encode_row_id(target, profile):
    return json.dumps([target, profile])


def _decode_row_id(encoded):
    return json.loads(encoded)
