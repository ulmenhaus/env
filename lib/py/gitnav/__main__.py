"""
gitview a curses interface for navigating git and github
"""

import os
import subprocess

import urwid

from git import Repo


class GitView(object):
    def __init__(self, repo, target_pane):
        self.repo = repo
        self.target_pane = target_pane
        self.main_view = None
        self.branch_view()

    def branch_view(self):
        # TODO preserve order
        others = list(self.repo.heads)
        others.remove(self.repo.head.reference)
        self.branches = [self.repo.head.reference] + others
        body = [urwid.Text("Branches"), urwid.Divider()]
        for ref in self.branches:
            button = urwid.Button(str(ref))
            urwid.connect_signal(button, 'click', self.branch_chosen, ref)
            body.append(button)

        walker = urwid.SimpleListWalker(body)
        self.main_view = urwid.ListBox(walker)
        urwid.connect_signal(walker, "modified", self.cursor_moved)

    def cursor_moved(self):
        pass

    def branch_chosen(self, button, choice):
        self._send_cmd("git checkout {}".format(str(choice)))
        raise urwid.ExitMainLoop()

    def global_input(self, key):
        if key in ('q', 'Q'):
            raise urwid.ExitMainLoop()
        # HACK 2 is the number of header elements?
        branch = self.branches[self.main_view.get_focus()[1] - 2]
        if key == 't':
            self._send_cmd("tig {}; tmux select-pane -t {}".format(
                branch.name, os.environ["TMUX_PANE"]))
            subprocess.check_call(
                ["tmux", "select-pane", "-t", self.target_pane])
        elif key == 's':
            self.sync_branch(branch)
        elif key == 'c':
            self._send_cmd("git checkout {}".format(branch.name))
        elif key == 'd':
            self._send_cmd("git branch -d {}".format(branch.name))

    def _send_cmd(self, cmd):
        subprocess.check_call(
            ["tmux", "send", "-t", self.target_pane, cmd, "ENTER"])

    def sync_branch(self, branch):
        # assumes the user's remote fork of the repo is called 'fork'
        tracking = branch.tracking_branch()
        if not tracking:
            self._send_cmd("git push --set-upstream fork {}".format(
                branch.name))
            return
        self._send_cmd(
            "git fetch {remote} {branch} && git rebase {remote}/{branch} {branch}".
            format(remote=tracking.remote_name, branch=branch.name))
        if tracking.remote_name == 'fork':
            self._send_cmd("git push fork {}".format(branch.name))

    def exit_program(button):
        raise urwid.ExitMainLoop()


def _find_git_repo(wd):
    if os.path.dirname(wd) == wd:
        raise Exception("Not in git repo")
    if os.path.exists(os.path.join(wd, ".git")):
        return wd
    return _find_git_repo(os.path.dirname(wd))


def main():
    if "GITNAV_PANE" not in os.environ:
        pane = os.environ["TMUX_PANE"]
        subprocess.check_call([
            "tmux", "split-window", "-p", "20", "-b", "-h", "bash", "-c",
            "cd {} && PYTHONPATH={} GITNAV_PANE={} python -m gitnav || sleep 30".
            format(os.getcwd(), os.environ.get("PYTHONPATH", ""), pane)
        ])
    else:
        repo = Repo(_find_git_repo(os.getcwd()))
        gv = GitView(repo, os.environ["GITNAV_PANE"])
        urwid.MainLoop(
            gv.main_view,
            unhandled_input=gv.global_input,
            palette=[('reversed', 'standout', '')]).run()


if __name__ == "__main__":
    main()
