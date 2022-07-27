import json
import os
import subprocess
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

    def _point_to_string(self, path, point):
        project = self._cache["projects"][self.project]
        projdir = os.path.abspath(os.path.expanduser(project["Workdir"]))
        relpath = os.path.abspath(path)[len(projdir) + 1:]
        return "{}#{}".format(relpath, point)

    def _save_cache(self):
        with open(self.path, 'w') as f:
            json.dump(self._cache, f, indent=4)
