import click

from euler import files
from euler import soln


@click.group()
def cli():
    pass


def main():
    # TODO(rabrams) I don't like specifying the whole command hierarchy at
    # the top like this
    @cli.group(name='soln')
    def soln_group():
        """
        Manage solutions to problems
        """
        pass

    soln_group.command()(soln.edit)
    soln_group.command()(soln.ls)
    soln_group.command()(soln.run)
    cli.command()(files.init)
    cli(obj={})


if __name__ == "__main__":
    main()
