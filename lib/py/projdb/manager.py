import json
import os
import subprocess
import sys
import tempfile


class ProjectManager(object):

    def __init__(self, path="~/.projects.json", project=""):
        self.path = os.path.expanduser(path)
        self.project = project
        with open(self.path) as f:
            self._cache = json.load(f)

    def new_project(self, workdir):
        reldir = os.path.abspath(workdir).replace(os.path.expanduser("~"), "~")
        self._cache["projects"][self.project] = {
            "Workdir": reldir,
            "Default Resource Filter": "",
        }
        self._save_cache()

    def list_projects(self):
        import tabulate
        table = [[
            name,
            project["Workdir"],
            project["Default Resource Filter"],
        ] for name, project in self._cache["projects"].items()]
        print(tabulate.tabulate(table))

    def open_runner(self, timedb_path, bin_path):
        """
        Opens the jql timedb runner with the project's desired
        """
        proj = self._cache["projects"].get(self.project)
        # HACK hard-coding localtion of runner
        args = ["/usr/local/bin/runner", timedb_path, bin_path]
        if proj and proj["Default Resource Filter"]:
            args.append(proj["Default Resource Filter"])
        os.execvp("/usr/local/bin/runner", args)

    def _prompt_for_description(self):
        with tempfile.NamedTemporaryFile() as tmp:
            # HACK hard-coding the location of prompt
            subprocess.check_call([
                "tmux", "popup", "-h", "8", "-E", "/usr/local/bin/prompt",
                "Describe your bookmark", tmp.name
            ])
            tmp.seek(0)
            return tmp.read().decode("utf-8")

    def add_bookmark(self, path, point):
        description = self._prompt_for_description()
        key = self._point_to_string(path, point)
        self._cache["bookmarks"][key] = {
            "Description": description,
            "Project": self.project,
        }
        self._save_cache()

    def clear_bookmarks(self):
        keys = list(self._cache["bookmarks"].keys())
        for key in keys:
            if self._cache["bookmarks"][key]['Project'] == self.project:
                del self._cache["bookmarks"][key]
        self._save_cache()

    def export_bookmarks(self):
        base_url = self._get_proj_base_url()
        for key, bookmark in self._cache["bookmarks"].items():
            if bookmark['Project'] != self.project:
                continue
            # NOTE this URL will go to the correct file path, but the location within it
            # is a character whereas github expects lines so won't be correct
            print(
                f"* [{bookmark['Description']}](https://{base_url}/blob/master/{key})"
            )

    def import_bookmarks(self):
        all_bookmarks = sys.stdin.read().split("\n")
        for bookmark in all_bookmarks:
            # NOTE this export/import is kinda lossy -- we assume we'll never have a description
            # with square braces or "/blob/master"
            if not bookmark.startswith("* ["):
                continue
            description, rest = bookmark[len("* ["):].split("](", maxsplit=1)
            key = rest.split("/blob/master/", maxsplit=1)[1][:-1]
            self._cache["bookmarks"][key] = {
                'Description': description,
                'Project': self.project,
            }
        self._save_cache()

    def _get_proj_base_url(self):
        proj_wd = self._cache["projects"][self.project]["Workdir"]
        raw_url = subprocess.check_output(
            "git config --get remote.origin.url",
            cwd=os.path.expanduser(proj_wd),
            shell=True,
        ).decode("utf-8").strip()
        if "@" not in raw_url:
            raise NotImplementedError(
                f"Exports not supported for http based origin url: {raw_url}")
        if raw_url.endswith(".git"):
            raw_url = raw_url[:-len(".git")]
        return raw_url.split("@")[1].replace(":", "/")

    def add_jump(self, source_path, source_point, target_path, target_point):
        all_jumps = self._cache["jumps"]
        filtered_jumps = {
            key: jump
            for key, jump in all_jumps.items()
            if jump["Project"] == self.project
        }
        other_jumps = {
            key: jump
            for key, jump in all_jumps.items() if key not in filtered_jumps
        }
        key = self._point_to_string(source_path, source_point)
        max_increment = filtered_jumps[key][
            "Order"] if key in filtered_jumps else 100
        to_del = [
            key for key, jump in filtered_jumps.items()
            if max_increment >= 100 and jump["Order"] >= 100
        ]
        for td in to_del:
            del filtered_jumps[td]
        for jump in filtered_jumps.values():
            if jump['Order'] <= max_increment:
                jump['Order'] += 1
        filtered_jumps[key] = {
            "A Target": self._point_to_string(target_path, target_point),
            "Order": 1,
            "Project": self.project,
        }
        filtered_jumps.update(
            other_jumps
        )  # unioning of dicts supported in python 3.9 for some future code golfing
        self._cache["jumps"] = filtered_jumps
        self._save_cache()

    def get_jumps(self):
        all_jumps = self._cache["jumps"]
        filtered_jumps = list(
            filter(lambda jump: jump["Project"] == self.project,
                   all_jumps.values()))
        return sorted(filtered_jumps, key=lambda jump: jump["Order"])

    def run_test(self, timedb_path, debug, focus):
        commands = self._get_project_commands(timedb_path)
        command = commands['Run project tests']
        if debug:
            command = commands['Debug project tests']
        if focus and focus != "-":
            command = commands["Run focused test"].format(focus=focus)
            if debug:
                command = commands["Debug focused test"].format(focus=focus)
        subprocess.check_call(
            ["tmux", "send", "-t", "right", command, "ENTER"])

    def _get_project_commands(self, timedb_path):
        proj = self._cache["projects"].get(self.project)
        resource = proj["Default Resource Filter"]
        with open(timedb_path) as f:
            timedb = json.load(f)
        commands = {}
        for assn in timedb['assertions'].values():
            if assn['A Relation'] != ".Command" or assn[
                    'Arg0'] != f"nouns {resource}":
                continue
            for line in assn['Arg1'].split("\n"):
                parts = line.split("|", 2)
                if len(parts) != 3:
                    continue
                _, desc, command = [part.strip() for part in parts]
                if not command.startswith("`"):
                    continue
                command = command.split("`")[1]
                commands[desc] = command
        return commands

    def _point_to_string(self, path, point):
        project = self._cache["projects"][self.project]
        projdir = os.path.abspath(os.path.expanduser(project["Workdir"]))
        relpath = os.path.abspath(path)[len(projdir) + 1:]
        return "{}#{}".format(relpath, point)

    def _save_cache(self):
        with open(self.path, 'w') as f:
            json.dump(self._cache, f, indent=4)
