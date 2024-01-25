from timedb.virtual_gateway import ideas, relatives, habituals

from jql import jql_pb2_grpc


class Gateway(jql_pb2_grpc.JQLServicer):

    def __init__(self, client):
        super().__init__()
        self.backends = {
            "vt.ideas": ideas.IdeasBackend(client),
            "vt.habituals": habituals.HabitualsBackend(client),
            "vt.relatives": relatives.RelativesBackend(client),
        }

    def _handle_request(self, request, context, method):
        for table, backend in self.backends.items():
            if not table.startswith(request.table):
                continue
            return getattr(backend, method)(request, context)
        raise ValueError("Unknown table", request.table)

    def ListRows(self, request, context):
        return self._handle_request(request, context, "ListRows")

    def IncrementEntry(self, request, context):
        return self._handle_request(request, context, "IncrementEntry")
