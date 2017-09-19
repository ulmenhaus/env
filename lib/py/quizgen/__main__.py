import click

from quizgen import scrape
from quizgen import stage
from quizgen import sync


@click.group()
def cli():
    pass


def main():
    cli.command(name='scrape')(scrape.scrape)
    cli.command(name='stage')(stage.stage)
    cli.command(name='sync')(sync.sync)
    cli(obj={})


if __name__ == "__main__":
    main()
