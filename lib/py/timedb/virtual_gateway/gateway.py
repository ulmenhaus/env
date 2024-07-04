from timedb.virtual_gateway import attributes, habituals, ideas, kits, practices, project_initiatives, relatives, review, tools

from jql import jql_pb2_grpc


class Gateway(jql_pb2_grpc.JQLServicer):

    def __init__(self, client):
        super().__init__()
        self.backends = {
            "vt.attributes": attributes.AttributesBackend(client),
            "vt.habituals": habituals.HabitualsBackend(client),
            "vt.ideas": ideas.IdeasBackend(client),
            "vt.kits": kits.KitsBackend(client),
            "vt.practices": practices.PracticesBackend(client),
            "vt.project_initiative_nouns": project_initiatives.NounsBackend(client),
            "vt.project_initiative_assertions": project_initiatives.AssertionsBackend(client),
            "vt.relatives": relatives.RelativesBackend(client),
            "vt.review": review.ReviewBackend(client),
            "vt.tools": tools.ToolsBackend(client),
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

    def WriteRow(self, request, context):
        return self._handle_request(request, context, "WriteRow")

    def DeleteRow(self, request, context):
        return self._handle_request(request, context, "DeleteRow")

    def GetRow(self, request, context):
        return self._handle_request(request, context, "GetRow")

