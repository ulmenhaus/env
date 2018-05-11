"""
rb: a runbook execution tool

rb will look through markdown files for command-line instructions to run and
create a side-bar for executing them in order
"""

import os
import subprocess
import sys

import urwid


class Interaction(object):
    def __init__(self, description, command):
        self.description = description
        self.command = command


class CommandView(object):
    def __init__(self, target_pane, title, ixns):
        self.target_pane = target_pane
        self.title = title
        self.ixns = ixns
        self.command_view()

    def command_view(self):
        body = [urwid.Text(self.title), urwid.Divider()]
        for ixn in self.ixns:
            button = urwid.Button(ixn.description)
            urwid.connect_signal(button, 'click', self.ixn_chosen, button)
            body.append(button)

        self.walker = urwid.SimpleListWalker(body)
        self.main_view = urwid.ListBox(self.walker)
        urwid.connect_signal(self.walker, "modified", self.cursor_moved)

    def cursor_moved(self):
        index = self.main_view.get_focus()[1]
        # HACK 2 is the number of header elements
        ixn = self.ixns[index - 2]
        self._set_input(ixn.command)

    def ixn_chosen(self, button, choice):
        subprocess.check_call(
            ["tmux", "send", "-t", self.target_pane, "ENTER"])

    def global_input(self, key):
        if key in ('q', 'Q'):
            raise urwid.ExitMainLoop()

    def _set_input(self, cmd):
        # HACK clear line with 100 backspaces
        subprocess.check_call(["tmux", "send", "-t", self.target_pane] +
                              ["C-h"] * 1000 + [cmd])


def _parse_md_file(path):
    # TODO consider using md library
    title = os.path.basename(path)
    title_set = False
    last_line = ""
    ixns = []
    with open(path) as f:
        for line in f:
            if line.startswith("# ") and not title_set:
                title = line[2:].strip()
                title_set = True
            elif line.startswith("```"):
                contents = ""
                for next_line in f:
                    if next_line.startswith("```"):
                        break
                    contents += next_line
                # TODO reduce multiple newlines to a single one
                contents = contents.strip().replace("\n", " && ")
                ixns.append(Interaction(last_line, contents))
            else:
                last_line = line.strip()
    return title, ixns


def main():
    if "RUNBOOK_PANE" not in os.environ:
        pane = os.environ["TMUX_PANE"]
        subprocess.check_call([
            "tmux", "split-window", "-p", "20", "-b", "-h", "bash", "-c",
            "cd {} && PYTHONPATH={} RUNBOOK_PANE={} python3 -m runbook {} || sleep 30".
            format(os.getcwd(),
                   os.environ.get("PYTHONPATH", ""), pane, sys.argv[1])
        ])
    else:
        title, ixns = _parse_md_file(sys.argv[1])
        cv = CommandView(os.environ["RUNBOOK_PANE"], title, ixns)
        urwid.MainLoop(
            cv.main_view,
            unhandled_input=cv.global_input,
            palette=[('reversed', 'standout', '')]).run()


if __name__ == "__main__":
    main()
