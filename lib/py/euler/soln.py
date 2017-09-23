import glob
import importlib
import json
import os

import click
import tabulate

from euler import files

SOLN_TEMPLATE = '''"""
"""


def get_answer():
    pass
'''


@click.argument('number', type=click.INT)
def edit(number):
    """
    Edit a particular solution

    NUMBER is the number of the problem
    """
    path = files.get_proj_dir_or_exit()
    editor = os.environ.get("EDITOR", "nano")
    fullpath = os.path.join(path, "solutions", "p{}.py".format(number))
    if not os.path.exists(fullpath):
        with open(fullpath, 'w') as f:
            f.write(SOLN_TEMPLATE)
    os.execvp(editor, [editor, fullpath])


def ls():
    """
    List the available solutions
    """
    # TODO(rabrams) would be good to factor out the file specific stuff
    # to files module
    path = files.get_proj_dir_or_exit()
    mod_paths = glob.glob(os.path.join(path, "solutions", "*.py"))
    basenames = [os.path.basename(mod_path) for mod_path in mod_paths]
    with open(os.path.join(path, "answers.json")) as f:
        answers = json.load(f)

    problem_data = {}
    for basename in basenames:
        if basename == "__init__.py":
            continue
        problem = basename[:-len(".py")]
        if not problem.startswith("p"):
            raise ValueError("Invalid module name found", basename)
        try:
            number = int(problem[1:])
        except ValueError:
            raise ValueError("Invalid module name found", basename)
        module = importlib.import_module("solutions.p{}".format(number))

        fulldoc = module.__doc__ or ""
        parts = fulldoc.split("\n")
        problem_data[number] = {
            "summary": "" if len(parts) <= 1 else parts[1].strip(),
            "answer": answers.get(str(number), ""),
        }

    table = {
        "problem": [],
        "summary": [],
        "answer": [],
    }
    for number in sorted(problem_data):
        table["problem"].append(number)
        table["summary"].append(problem_data[number]['summary'])
        table["answer"].append(problem_data[number]['answer'])
    print(tabulate.tabulate(table, headers="keys"))


@click.argument('number', type=click.INT)
@click.option(
    '--quiet', is_flag=True, default=False, help='output just the solution')
def run(number, quiet):
    """
    Run a particular solution

    NUMBER is the number of the problem
    """
    key = str(number)
    path = files.get_proj_dir_or_exit()
    answers_path = os.path.join(path, "answers.json")
    with open(answers_path) as f:
        answers = json.load(f)
    module = importlib.import_module("solutions.p{}".format(number))
    answer = module.get_answer()
    if not isinstance(answer, (int, str)):
        raise TypeError("Invalid type for answer", type(answer))
    status = "Adding new entry"
    if key in answers:
        if answers[key] == answer:
            status = "Entry matches"
        else:
            status = "Entry does not matches"
    if quiet:
        print(answer)
    else:
        print("Answer: {}".format(answer))
        print("Status: {}".format(status))
    answers[key] = answer
    with open(answers_path, 'w') as f:
        json.dump(answers, f, indent=4, sort_keys=True)
