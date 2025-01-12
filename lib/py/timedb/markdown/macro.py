import subprocess
import sys

from jql import macro
from timedb import schema


from timedb.virtual_gateway import common

class MacroHandler(object):
    def _normalize_pk(self, table, pk):
        if table == schema.Tables.Attributes:
            assn_pk, attrs = common.decode_pk(pk)
            item_pk = attrs[schema.Fields.Item][0]
            return item_pk, attrs[schema.Fields.NounRelation][0]
        elif table == schema.Tables.Relatives:
            pk, attrs = common.decode_pk(pk)
        return f"{table} {pk}", ".Note"

    def run(self, split):
        with macro.macro_interface(sys.stdin, sys.stdout) as iface:
            dbms = iface.get_dbms()
            table, pk = iface.get_primary_selection()
        full_pk, field = self._normalize_pk(table, pk)
        subprocess.Popen(["tmux", "select-pane", "-R"]).wait()
        prefix = "send-left "
        if not split:
            subprocess.Popen(["tmux", "resize-pane", "-Z"]).wait()
            prefix = ""
        subprocess.Popen(["tmux", "send", f"{prefix}{field}\t{full_pk}", "ENTER"]).wait()
