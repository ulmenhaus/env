import click

from googul import reminder


@click.group()
def cli():
    pass


def main():
    # TODO(rabrams) I don't like specifying the whole command hierarchy at
    # the top like this
    @cli.group(name='reminder')
    def reminder_group():
        """
        Manage google reminders
        """
        pass

    reminder_group.command()(reminder.ls)
    cli(obj={})


if __name__ == "__main__":
    main()
