import contextlib
import json

import grpc

from jql import jql_pb2_grpc


class MacroInterface(object):

    def __init__(self, f):
        self.attrs = json.load(f)
        self.snapshot = json.loads(
            self.attrs["snapshot"]) if self.attrs["snapshot"] else {}

    def get_dbms(self):
        if self.attrs["snapshot"] and self.attrs["address"]:
            raise ValueError(
                "Macro interface cannot have both snapshot and address set")
        if self.attrs["address"]:
            return jql_pb2_grpc.JQLStub(grpc.insecure_channel(self.attrs["address"]))
        elif self.attrs["snapshot"]:
            return JQLShim(self.attrs["snapshot"])
        else:
            raise ValueError(
                "macro interface must have either snapshot or address set")

    def get_primary_selection(self):
        return self.attrs["current_view"]["table"], self.attrs["current_view"]["primary_selection"]

    def call_back(self, f):
        if self.attrs["snapshot"]:
            self.attrs["snapshot"] = json.dumps(self.snapshot)
        json.dump(self.attrs, f)


@contextlib.contextmanager
def macro_interface(i, o):
    iface = MacroInterface(i)
    yield iface
    iface.call_back(o)


class JQLShim(object):

    def __init__(self, snapshot):
        raise NotImplementedError("JQL shim not yet implemented")
