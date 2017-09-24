import click

from mixup import build


@click.group()
def cli():
    pass


def main():
    cli.command(name='build')(build.build)
    cli(obj={})


if __name__ == "__main__":
    main()
