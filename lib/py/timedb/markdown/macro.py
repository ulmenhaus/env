import subprocess
import sys

from jql import macro


class MacroHandler(object):
    def run(self):
        with macro.macro_interface(sys.stdin, sys.stdout) as iface:
            dbms = iface.get_dbms()
            table, pk = iface.get_primary_selection()
        subprocess.Popen(["tmux", "select-pane", "-R"]).wait()
        subprocess.Popen(["tmux", "resize-pane", "-Z"]).wait()
        subprocess.Popen(["tmux", "send", f"{table} {pk}", "ENTER"]).wait()
