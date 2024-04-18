import contextlib
import datetime
import json

import grpc

from jql import jql_pb2_grpc, jql_pb2


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
            return jql_pb2_grpc.JQLStub(
                grpc.insecure_channel(self.attrs["address"]))
        elif self.attrs["snapshot"]:
            return JQLShim(self.attrs["snapshot"])
        else:
            raise ValueError(
                "macro interface must have either snapshot or address set")

    def get_primary_selection(self):
        return self.attrs["current_view"]["table"], self.attrs["current_view"][
            "primary_selection"]

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


def proto_to_dict(columns, row):
    d = {}
    for i, col in enumerate(columns):
        if col.type == jql_pb2.EntryType.DATE:
            parsed = datetime.datetime.strptime(row.entries[i].formatted,
                                                "%d %b %Y")
            delta = parsed - datetime.datetime(1970, 1, 1)
            d[col.name] = int(delta.days)
        elif col.type == jql_pb2.EntryType.INT:
            d[col.name] = int(row.entries[i].formatted)
        elif col.type == jql_pb2.EntryType.TIME:
            raise NotImplementedError(
                "conversion from time types not supported")
        else:
            d[col.name] = row.entries[i].formatted
    return d


def protos_to_dict(columns, rows):
    ds = {}
    primary, = [i for i, c in enumerate(columns) if c.primary]
    for row in rows:
        ds[row.entries[primary].formatted] = proto_to_dict(columns, row)
    return ds
