"""
The arduino chipset prototyper
"""

import click

from acp import arch


def main():
    # TODO figure out how to show "acp" and description in the main help text
    arch.cli(obj={})


if __name__ == "__main__":
    main()
