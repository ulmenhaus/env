# Generated by the gRPC Python protocol compiler plugin. DO NOT EDIT!
"""Client and server classes corresponding to protobuf-defined services."""
import grpc
import warnings

from jql import jql_pb2 as jql_dot_jql__pb2

GRPC_GENERATED_VERSION = '1.70.0'
GRPC_VERSION = grpc.__version__
_version_not_supported = False

try:
    from grpc._utilities import first_version_is_lower
    _version_not_supported = first_version_is_lower(GRPC_VERSION, GRPC_GENERATED_VERSION)
except ImportError:
    _version_not_supported = True

if _version_not_supported:
    raise RuntimeError(
        f'The grpc package installed is at version {GRPC_VERSION},'
        + f' but the generated code in jql/jql_pb2_grpc.py depends on'
        + f' grpcio>={GRPC_GENERATED_VERSION}.'
        + f' Please upgrade your grpc module to grpcio>={GRPC_GENERATED_VERSION}'
        + f' or downgrade your generated code using grpcio-tools<={GRPC_VERSION}.'
    )


class JQLStub(object):
    """Missing associated documentation comment in .proto file."""

    def __init__(self, channel):
        """Constructor.

        Args:
            channel: A grpc.Channel.
        """
        self.ListTables = channel.unary_unary(
                '/jql.JQL/ListTables',
                request_serializer=jql_dot_jql__pb2.ListTablesRequest.SerializeToString,
                response_deserializer=jql_dot_jql__pb2.ListTablesResponse.FromString,
                _registered_method=True)
        self.ListRows = channel.unary_unary(
                '/jql.JQL/ListRows',
                request_serializer=jql_dot_jql__pb2.ListRowsRequest.SerializeToString,
                response_deserializer=jql_dot_jql__pb2.ListRowsResponse.FromString,
                _registered_method=True)
        self.GetRow = channel.unary_unary(
                '/jql.JQL/GetRow',
                request_serializer=jql_dot_jql__pb2.GetRowRequest.SerializeToString,
                response_deserializer=jql_dot_jql__pb2.GetRowResponse.FromString,
                _registered_method=True)
        self.WriteRow = channel.unary_unary(
                '/jql.JQL/WriteRow',
                request_serializer=jql_dot_jql__pb2.WriteRowRequest.SerializeToString,
                response_deserializer=jql_dot_jql__pb2.WriteRowResponse.FromString,
                _registered_method=True)
        self.DeleteRow = channel.unary_unary(
                '/jql.JQL/DeleteRow',
                request_serializer=jql_dot_jql__pb2.DeleteRowRequest.SerializeToString,
                response_deserializer=jql_dot_jql__pb2.DeleteRowResponse.FromString,
                _registered_method=True)
        self.IncrementEntry = channel.unary_unary(
                '/jql.JQL/IncrementEntry',
                request_serializer=jql_dot_jql__pb2.IncrementEntryRequest.SerializeToString,
                response_deserializer=jql_dot_jql__pb2.IncrementEntryResponse.FromString,
                _registered_method=True)
        self.Persist = channel.unary_unary(
                '/jql.JQL/Persist',
                request_serializer=jql_dot_jql__pb2.PersistRequest.SerializeToString,
                response_deserializer=jql_dot_jql__pb2.PersistResponse.FromString,
                _registered_method=True)
        self.GetSnapshot = channel.unary_unary(
                '/jql.JQL/GetSnapshot',
                request_serializer=jql_dot_jql__pb2.GetSnapshotRequest.SerializeToString,
                response_deserializer=jql_dot_jql__pb2.GetSnapshotResponse.FromString,
                _registered_method=True)
        self.LoadSnapshot = channel.unary_unary(
                '/jql.JQL/LoadSnapshot',
                request_serializer=jql_dot_jql__pb2.LoadSnapshotRequest.SerializeToString,
                response_deserializer=jql_dot_jql__pb2.LoadSnapshotResponse.FromString,
                _registered_method=True)


class JQLServicer(object):
    """Missing associated documentation comment in .proto file."""

    def ListTables(self, request, context):
        """Missing associated documentation comment in .proto file."""
        context.set_code(grpc.StatusCode.UNIMPLEMENTED)
        context.set_details('Method not implemented!')
        raise NotImplementedError('Method not implemented!')

    def ListRows(self, request, context):
        """Missing associated documentation comment in .proto file."""
        context.set_code(grpc.StatusCode.UNIMPLEMENTED)
        context.set_details('Method not implemented!')
        raise NotImplementedError('Method not implemented!')

    def GetRow(self, request, context):
        """Missing associated documentation comment in .proto file."""
        context.set_code(grpc.StatusCode.UNIMPLEMENTED)
        context.set_details('Method not implemented!')
        raise NotImplementedError('Method not implemented!')

    def WriteRow(self, request, context):
        """Missing associated documentation comment in .proto file."""
        context.set_code(grpc.StatusCode.UNIMPLEMENTED)
        context.set_details('Method not implemented!')
        raise NotImplementedError('Method not implemented!')

    def DeleteRow(self, request, context):
        """Missing associated documentation comment in .proto file."""
        context.set_code(grpc.StatusCode.UNIMPLEMENTED)
        context.set_details('Method not implemented!')
        raise NotImplementedError('Method not implemented!')

    def IncrementEntry(self, request, context):
        """Missing associated documentation comment in .proto file."""
        context.set_code(grpc.StatusCode.UNIMPLEMENTED)
        context.set_details('Method not implemented!')
        raise NotImplementedError('Method not implemented!')

    def Persist(self, request, context):
        """Missing associated documentation comment in .proto file."""
        context.set_code(grpc.StatusCode.UNIMPLEMENTED)
        context.set_details('Method not implemented!')
        raise NotImplementedError('Method not implemented!')

    def GetSnapshot(self, request, context):
        """Missing associated documentation comment in .proto file."""
        context.set_code(grpc.StatusCode.UNIMPLEMENTED)
        context.set_details('Method not implemented!')
        raise NotImplementedError('Method not implemented!')

    def LoadSnapshot(self, request, context):
        """Missing associated documentation comment in .proto file."""
        context.set_code(grpc.StatusCode.UNIMPLEMENTED)
        context.set_details('Method not implemented!')
        raise NotImplementedError('Method not implemented!')


def add_JQLServicer_to_server(servicer, server):
    rpc_method_handlers = {
            'ListTables': grpc.unary_unary_rpc_method_handler(
                    servicer.ListTables,
                    request_deserializer=jql_dot_jql__pb2.ListTablesRequest.FromString,
                    response_serializer=jql_dot_jql__pb2.ListTablesResponse.SerializeToString,
            ),
            'ListRows': grpc.unary_unary_rpc_method_handler(
                    servicer.ListRows,
                    request_deserializer=jql_dot_jql__pb2.ListRowsRequest.FromString,
                    response_serializer=jql_dot_jql__pb2.ListRowsResponse.SerializeToString,
            ),
            'GetRow': grpc.unary_unary_rpc_method_handler(
                    servicer.GetRow,
                    request_deserializer=jql_dot_jql__pb2.GetRowRequest.FromString,
                    response_serializer=jql_dot_jql__pb2.GetRowResponse.SerializeToString,
            ),
            'WriteRow': grpc.unary_unary_rpc_method_handler(
                    servicer.WriteRow,
                    request_deserializer=jql_dot_jql__pb2.WriteRowRequest.FromString,
                    response_serializer=jql_dot_jql__pb2.WriteRowResponse.SerializeToString,
            ),
            'DeleteRow': grpc.unary_unary_rpc_method_handler(
                    servicer.DeleteRow,
                    request_deserializer=jql_dot_jql__pb2.DeleteRowRequest.FromString,
                    response_serializer=jql_dot_jql__pb2.DeleteRowResponse.SerializeToString,
            ),
            'IncrementEntry': grpc.unary_unary_rpc_method_handler(
                    servicer.IncrementEntry,
                    request_deserializer=jql_dot_jql__pb2.IncrementEntryRequest.FromString,
                    response_serializer=jql_dot_jql__pb2.IncrementEntryResponse.SerializeToString,
            ),
            'Persist': grpc.unary_unary_rpc_method_handler(
                    servicer.Persist,
                    request_deserializer=jql_dot_jql__pb2.PersistRequest.FromString,
                    response_serializer=jql_dot_jql__pb2.PersistResponse.SerializeToString,
            ),
            'GetSnapshot': grpc.unary_unary_rpc_method_handler(
                    servicer.GetSnapshot,
                    request_deserializer=jql_dot_jql__pb2.GetSnapshotRequest.FromString,
                    response_serializer=jql_dot_jql__pb2.GetSnapshotResponse.SerializeToString,
            ),
            'LoadSnapshot': grpc.unary_unary_rpc_method_handler(
                    servicer.LoadSnapshot,
                    request_deserializer=jql_dot_jql__pb2.LoadSnapshotRequest.FromString,
                    response_serializer=jql_dot_jql__pb2.LoadSnapshotResponse.SerializeToString,
            ),
    }
    generic_handler = grpc.method_handlers_generic_handler(
            'jql.JQL', rpc_method_handlers)
    server.add_generic_rpc_handlers((generic_handler,))
    server.add_registered_method_handlers('jql.JQL', rpc_method_handlers)


 # This class is part of an EXPERIMENTAL API.
class JQL(object):
    """Missing associated documentation comment in .proto file."""

    @staticmethod
    def ListTables(request,
            target,
            options=(),
            channel_credentials=None,
            call_credentials=None,
            insecure=False,
            compression=None,
            wait_for_ready=None,
            timeout=None,
            metadata=None):
        return grpc.experimental.unary_unary(
            request,
            target,
            '/jql.JQL/ListTables',
            jql_dot_jql__pb2.ListTablesRequest.SerializeToString,
            jql_dot_jql__pb2.ListTablesResponse.FromString,
            options,
            channel_credentials,
            insecure,
            call_credentials,
            compression,
            wait_for_ready,
            timeout,
            metadata,
            _registered_method=True)

    @staticmethod
    def ListRows(request,
            target,
            options=(),
            channel_credentials=None,
            call_credentials=None,
            insecure=False,
            compression=None,
            wait_for_ready=None,
            timeout=None,
            metadata=None):
        return grpc.experimental.unary_unary(
            request,
            target,
            '/jql.JQL/ListRows',
            jql_dot_jql__pb2.ListRowsRequest.SerializeToString,
            jql_dot_jql__pb2.ListRowsResponse.FromString,
            options,
            channel_credentials,
            insecure,
            call_credentials,
            compression,
            wait_for_ready,
            timeout,
            metadata,
            _registered_method=True)

    @staticmethod
    def GetRow(request,
            target,
            options=(),
            channel_credentials=None,
            call_credentials=None,
            insecure=False,
            compression=None,
            wait_for_ready=None,
            timeout=None,
            metadata=None):
        return grpc.experimental.unary_unary(
            request,
            target,
            '/jql.JQL/GetRow',
            jql_dot_jql__pb2.GetRowRequest.SerializeToString,
            jql_dot_jql__pb2.GetRowResponse.FromString,
            options,
            channel_credentials,
            insecure,
            call_credentials,
            compression,
            wait_for_ready,
            timeout,
            metadata,
            _registered_method=True)

    @staticmethod
    def WriteRow(request,
            target,
            options=(),
            channel_credentials=None,
            call_credentials=None,
            insecure=False,
            compression=None,
            wait_for_ready=None,
            timeout=None,
            metadata=None):
        return grpc.experimental.unary_unary(
            request,
            target,
            '/jql.JQL/WriteRow',
            jql_dot_jql__pb2.WriteRowRequest.SerializeToString,
            jql_dot_jql__pb2.WriteRowResponse.FromString,
            options,
            channel_credentials,
            insecure,
            call_credentials,
            compression,
            wait_for_ready,
            timeout,
            metadata,
            _registered_method=True)

    @staticmethod
    def DeleteRow(request,
            target,
            options=(),
            channel_credentials=None,
            call_credentials=None,
            insecure=False,
            compression=None,
            wait_for_ready=None,
            timeout=None,
            metadata=None):
        return grpc.experimental.unary_unary(
            request,
            target,
            '/jql.JQL/DeleteRow',
            jql_dot_jql__pb2.DeleteRowRequest.SerializeToString,
            jql_dot_jql__pb2.DeleteRowResponse.FromString,
            options,
            channel_credentials,
            insecure,
            call_credentials,
            compression,
            wait_for_ready,
            timeout,
            metadata,
            _registered_method=True)

    @staticmethod
    def IncrementEntry(request,
            target,
            options=(),
            channel_credentials=None,
            call_credentials=None,
            insecure=False,
            compression=None,
            wait_for_ready=None,
            timeout=None,
            metadata=None):
        return grpc.experimental.unary_unary(
            request,
            target,
            '/jql.JQL/IncrementEntry',
            jql_dot_jql__pb2.IncrementEntryRequest.SerializeToString,
            jql_dot_jql__pb2.IncrementEntryResponse.FromString,
            options,
            channel_credentials,
            insecure,
            call_credentials,
            compression,
            wait_for_ready,
            timeout,
            metadata,
            _registered_method=True)

    @staticmethod
    def Persist(request,
            target,
            options=(),
            channel_credentials=None,
            call_credentials=None,
            insecure=False,
            compression=None,
            wait_for_ready=None,
            timeout=None,
            metadata=None):
        return grpc.experimental.unary_unary(
            request,
            target,
            '/jql.JQL/Persist',
            jql_dot_jql__pb2.PersistRequest.SerializeToString,
            jql_dot_jql__pb2.PersistResponse.FromString,
            options,
            channel_credentials,
            insecure,
            call_credentials,
            compression,
            wait_for_ready,
            timeout,
            metadata,
            _registered_method=True)

    @staticmethod
    def GetSnapshot(request,
            target,
            options=(),
            channel_credentials=None,
            call_credentials=None,
            insecure=False,
            compression=None,
            wait_for_ready=None,
            timeout=None,
            metadata=None):
        return grpc.experimental.unary_unary(
            request,
            target,
            '/jql.JQL/GetSnapshot',
            jql_dot_jql__pb2.GetSnapshotRequest.SerializeToString,
            jql_dot_jql__pb2.GetSnapshotResponse.FromString,
            options,
            channel_credentials,
            insecure,
            call_credentials,
            compression,
            wait_for_ready,
            timeout,
            metadata,
            _registered_method=True)

    @staticmethod
    def LoadSnapshot(request,
            target,
            options=(),
            channel_credentials=None,
            call_credentials=None,
            insecure=False,
            compression=None,
            wait_for_ready=None,
            timeout=None,
            metadata=None):
        return grpc.experimental.unary_unary(
            request,
            target,
            '/jql.JQL/LoadSnapshot',
            jql_dot_jql__pb2.LoadSnapshotRequest.SerializeToString,
            jql_dot_jql__pb2.LoadSnapshotResponse.FromString,
            options,
            channel_credentials,
            insecure,
            call_credentials,
            compression,
            wait_for_ready,
            timeout,
            metadata,
            _registered_method=True)
