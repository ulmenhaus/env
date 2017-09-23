import os
import sys

DIR_NAME = "PROJECT_EULER_DIR"
SUBPATHS = ("utils", "data", "solutions")


def _exit_with_msg(msg, code=1):
    print(msg, file=sys.stderr)
    exit(code)


def get_proj_dir_or_exit():
    if DIR_NAME not in os.environ:
        msg = "Please specify a project directory with the {} env var".format(
            DIR_NAME)
        _exit_with_msg(msg)
    path = os.environ[DIR_NAME]
    if not os.path.exists(path):
        msg = "Proj dir not does not exist {}".format(path)
        _exit_with_msg(msg)
    for subpath in SUBPATHS:
        if not os.path.exists(os.path.join(path, subpath)):
            msg = "Proj dir not initialized -- missing {}".format(subpath)
            _exit_with_msg(msg)
    if not os.path.exists(os.path.join(path, "answers.json")):
        msg = "Proj dir not initialized -- missing {}".format("answers.json")
        _exit_with_msg(msg)
    sys.path.insert(0, path)
    return path


def init():
    """
    Initialize project euler directory
    """
    if DIR_NAME not in os.environ:
        msg = "Please specify a project directory with the {} env var".format(
            DIR_NAME)
        _exit_with_msg(msg)
    path = os.environ[DIR_NAME]
    if not os.path.exists(path):
        msg = "Proj dir not does not exist {}".format(path)
        _exit_with_msg(msg)
    for subpath in SUBPATHS:
        os.mkdir(os.path.join(path, subpath))
        with open(os.path.join(path, subpath, "__init__.py"), 'w') as f:
            f.write("")
    with open(os.path.join(path, "answers.json"), 'w') as f:
        f.write("{}")
