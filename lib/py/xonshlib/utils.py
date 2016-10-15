import json
import os
import subprocess

from xonshlib import ENV


class PassContextManager(object):
    def __init__(self, pass_stores):
        self.pass_stores = pass_stores
        self.reverse_stores = {self.combine(value): key
                               for key, value in pass_stores.items()}

    @staticmethod
    def combine(pathish):
        return pathish if isinstance(pathish, str) else os.path.join(*pathish)

    def get_context(self):
        return self.reverse_stores.get(ENV.get('PASSWORD_STORE_DIR'), 'none')

    def set_context(self, args, stdin=None):
        name, = args
        if name == '-':
            if "PASSWORD_STORE_DIR" in ENV:
                del ENV['PASSWORD_STORE_DIR']
            if "PASSWORD_STORE_GIT" in ENV:
                del ENV['PASSWORD_STORE_GIT']
        else:
            store = self.pass_stores[args[0]]
            if isinstance(store, str):
                ENV['PASSWORD_STORE_DIR'] = store
                ENV['PASSWORD_STORE_GIT'] = store
            else:
                git_dir, store_dir = store
                ENV['PASSWORD_STORE_DIR'] = os.path.join(git_dir, store_dir)
                ENV['PASSWORD_STORE_GIT'] = git_dir

    def complete_line(self, prefix, line, begidx, endidx, ctx):
        if not line.startswith("pass"):
            return

        if "PASSWORD_STORE_DIR" not in ENV:
            return {"No pass context", ""}

        parts = os.path.split(prefix)
        path_so_far = os.path.join(*parts[:-1])
        subdir = os.path.join(ENV["PASSWORD_STORE_DIR"], path_so_far)
        all_files = os.listdir(subdir)
        process_filename = lambda filename : os.path.join(filename, "") \
                           if not filename.endswith(".gpg") \
                           else filename[:-4]
        is_match = lambda filename: filename.startswith(parts[-1])
        completions = map(process_filename, filter(is_match, all_files))
        return {os.path.join(path_so_far, completion)
                for completion in completions}


def docker_machine_env(args, stdin=None):
    name, = args
    if name == '-':
        for key in list(ENV):
            if key.startswith("DOCKER_"):
                del ENV[key]
        return

    output = subprocess.check_output(['docker-machine', 'env', name])
    for line in output.decode('utf-8').split("\n"):
        if line.startswith("export "):
            cmd = line[len("export "):]
            key, value = cmd.split("=")
            ENV[key] = json.loads(value)
    ENV["DOCKER_MACHINE"] = name


def command_for_container_config(config):
    volumes = " ".join("-v {}:{}".format(key, value)
                       for key, value in config.get("volumes", {}).items())
    return "docker run {it} {rm} {privileged} {net} {volumes} {image}".format(
        it=("-it" if config.get("it") else ""),
        rm=("--rm" if config.get("rm") else ""),
        net=("--net {}".format(config['net']) if "net" in config else ""),
        privileged=("--privileged" if config.get("privileged") else ""),
        volumes=volumes,
        image=config['image'], )


def docker_clean(args, stdin=None):
    output = subprocess.check_output(['docker', 'ps', '-aq'])
    cids = list(output.decode("utf-8").split("\n")[:-1])
    if not cids:
        return
    command = ["docker", "rm", "-f"] + cids
    subprocess.check_output(command)


# TODO make into a binary so that "git br-clean" will also work and can be invoked en
# masse using gr
def git_clean(args, stdin=None):
    branches = subprocess.check_output(["git", "branch"]).decode(
        "utf-8").split("\n")
    other_branches = []
    current_branch = None
    for branchline in branches:
        branch = branchline.strip(" ")
        if branch.startswith("* "):
            current_branch = branch[2:]
        elif branch:
            other_branches.append(branch)

    if not current_branch:
        print("No current branch found")
    elif current_branch != 'master':
        print("fatal: trying to run clean from a branch other than master")
    elif not other_branches:
        return
    else:
        subprocess.call(["git", "branch", "-d"] + other_branches)
