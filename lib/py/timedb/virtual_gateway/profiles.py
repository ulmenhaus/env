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
        return common.list_rows('vt.profiles', {}, request)

    def IncrementEntry(self, request, context):
        return self._handle_request(request, context, "IncrementEntry")

    def WriteRow(self, request, context):
        return self._handle_request(request, context, "WriteRow")
