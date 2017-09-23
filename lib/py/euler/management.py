import contextlib
import importlib
import json
import os
import subprocess

import euler


PROJECT_DIR = os.path.dirname(euler.__file__)
ANSWERS_PATH = os.path.join(PROJECT_DIR, "answers.json")
SOLUTIONS_PATH = os.path.join(PROJECT_DIR, "solutions")

SOLUTION_TEMPLATE = """
def get_answer():
    pass
"""


def get_answers():
    with open(ANSWERS_PATH) as f:
        return {int(key): value for key, value in json.load(f).items()}


def save_answers(answers):
    with open(ANSWERS_PATH, 'w') as f:
        json.dump(answers, f, sort_keys=True, indent=4)


@contextlib.contextmanager
def update_answers():
    current_answers = get_answers()
    yield current_answers
    save_answers(current_answers)


def new_solution_template(number):
    solution_path = os.path.join(SOLUTIONS_PATH, "p%s.py" % number)
    with open(solution_path, 'w') as f:
        f.write(SOLUTION_TEMPLATE)

    subprocess.check_call(["git", "add", solution_path])

    with update_answers() as answers:
        if number not in answers:
            answers[number] = None

    # TODO consider getting EDITOR from env
    os.execv("/usr/local/bin/emacs", ["emacs", solution_path])


def get_solution_module(number):
    return importlib.import_module("euler.solutions.p%s" % number)
