import click

from googul import translate


@click.group()
def cli():
    pass


def main():
    cli.command()(translate.translate)
    cli(obj={})


if __name__ == "__main__":
    main()
