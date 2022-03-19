import os
import tempfile

import click
import subprocess
import tabulate


class Architecture(object):
    pass


# TODO technically metaclass so figure out how that work
class Arch6502(Architecture):
    name = "6502"
    # NOTE the architecture here shold be generic enough to work with any 16 bit address
    # bus and 8 bit data bus, but the template will need to be generalized to support different
    # memory maps
    description = "Provide simulated RAM and ROM for a MOS Technology 6502 - requires a memory table"

    def run(fqbn, port, mem, log_level):
        tmpdir = tempfile.TemporaryDirectory()
        proj_dir = os.path.join(tmpdir.name, "Project")
        os.mkdir(proj_dir)
        template_path = os.path.join(os.path.dirname(__file__),
                                     "6502_template.ino")
        with open(template_path) as f:
            template = f.read()
        with open(mem, 'rb') as f:
            mem_table = f.read()
        if len(mem_table) > 0x8000 - 2:
            raise ValuError("Can't fit ROM in 6502 address space")
        int_vals = ", ".join(map(str, mem_table))
        output = template.replace("{MEMORY_ARRAY}", int_vals)
        output = output.replace("{ROM_LENGTH}", str(len(mem_table)))
        output = output.replace("{LOG_DEBUG}", "true" if log_level == "debug" else "false")
        with open(os.path.join(proj_dir, "Project.ino"), 'w') as f:
            f.write(output)
        os.chdir(tmpdir.name)
        subprocess.check_call(
            ["arduino-cli", "compile", "--fqbn", fqbn, "Project"])
        subprocess.check_call(
            ["arduino-cli", "upload", "-p", port, "--fqbn", fqbn, "Project"])


ARCHS = [Arch6502]


@click.group()
def cli():
    """
    acp: The Arduino Chipset Prototyper

    acp is a tool that makes it possible to simulate various subsets of a
    computer chipset for rapid prototyping and testing
    """
    pass


@cli.group(name='arch')
def arch_group():
    """
    Get info about the supported architectures
    """
    pass


@arch_group.command()
def ls():
    """
    List the supported architectures
    """
    table = {
        "name": [arch.name for arch in ARCHS],
        "description": [arch.description for arch in ARCHS],
    }
    print(tabulate.tabulate(table, headers="keys"))


@cli.command()
@click.option('--target', default='6502', type=click.STRING)
@click.option('--fqbn', default='arduino:avr:mega', type=click.STRING)
@click.option('--port', default='/dev/ttyACM0', type=click.STRING)
@click.option('--log-level', default='info', type=click.STRING)
# TODO mem may not be applicable to all target arches
@click.argument('mem', default="a.out", type=click.STRING)
def run(target, fqbn, mem, port, log_level):
    """
    Compile and upload a program to an arduino for simluating
    chipset components
    """
    arch_by_name = {arch.name: arch for arch in ARCHS}
    arch_by_name[target].run(fqbn, port, mem, log_level)
