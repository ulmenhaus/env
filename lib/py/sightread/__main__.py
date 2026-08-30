import argparse

from sightread import generator, ui


def main():
    parser = argparse.ArgumentParser(
        prog="sightread",
        description="Daily guitar sight-reading practice: randomly generated "
                     "melodies and triad studies, rendered as staff notation "
                     "and played back over MIDI.",
    )
    parser.add_argument(
        "-l", "--level", type=int, default=None,
        help=f"start at this level (1-{generator.NUM_LEVELS}); "
             "defaults to the last level you practiced",
    )
    args = parser.parse_args()

    if args.level is not None and not (1 <= args.level <= generator.NUM_LEVELS):
        parser.error(f"--level must be between 1 and {generator.NUM_LEVELS}")

    ui.main(initial_level=args.level)


if __name__ == "__main__":
    main()
