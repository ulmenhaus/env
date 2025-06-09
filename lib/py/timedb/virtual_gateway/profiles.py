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
        profiles = common.find_matching_auxiliaries(self.client, targets, "Schema")
        distinct_profiles = list(set().union(*profiles.values()))
        profile_fields, _ = common.get_fields_for_items(self.client, "", distinct_profiles, fields=['Dimension'])
        target_fields, _ = common.get_fields_for_items(self.client, "", targets)
        rows = {}
        for target, matching_profiles in profiles.items():
            for profile in matching_profiles:
                pk = str((profile, target))
                rows[pk] = {
                    "_pk": pk,
                    "_target": target,
                    "pk": [f"@{{{target}}}"],
                    "Profile": [profile],
                }
                for field in profile_fields[profile]['Dimension']:
                    rows[pk][field] = []
                for field, values in target_fields[target].items():
                    if field in profile_fields[profile]['Dimension']:
                        rows[pk][field] = values
        return common.list_rows('vt.profiles', rows, request)



    def IncrementEntry(self, request, context):
        return self._handle_request(request, context, "IncrementEntry")

    def WriteRow(self, request, context):
        return self._handle_request(request, context, "WriteRow")
