import argparse
import sys

from languify import config as config_mod
from languify import ocr, tts


def _add_language_arg(parser: argparse.ArgumentParser) -> None:
    parser.add_argument(
        "-l", "--language", required=True,
        help="language to process: farsi or hebrew (aliases: fa/fas/persian, he/heb)",
    )


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="languify",
        description="OCR and text-to-speech for Hebrew and Farsi, backed by "
                    "models configured in ~/.languify.json (see languify_setup.txt).",
    )
    parser.add_argument(
        "--config", default=None,
        help="path to the languify config JSON (default: ~/.languify.json)",
    )
    sub = parser.add_subparsers(dest="command", required=True)

    ocr_parser = sub.add_parser("ocr", help="OCR a batch of images into text files")
    _add_language_arg(ocr_parser)
    ocr_parser.add_argument("images", nargs="+", help="image files to OCR")
    ocr_parser.add_argument(
        "-o", "--out-dir", default=None,
        help="directory to write <image>.txt files into (default: alongside each image)",
    )

    tts_parser = sub.add_parser("tts", help="text-to-speech for a batch of text files")
    _add_language_arg(tts_parser)
    tts_parser.add_argument(
        "texts", nargs="+",
        help="text files to read ('-' pastes from the clipboard via txtpaste)",
    )
    tts_parser.add_argument(
        "--out", default=None,
        help="combined output audio path; required unless exactly one text "
             "file (not the clipboard) is given (default mode only)",
    )
    tts_parser.add_argument(
        "-i", "--interactive", action="store_true",
        help="play back interactively in a curses reader instead of "
             "writing a combined audio file ('-' sources can be re-pasted "
             "with enter)",
    )

    return parser


def main(argv=None) -> None:
    args = build_parser().parse_args(argv)

    try:
        lang = config_mod.get_language_config(args.language, args.config)
        if args.command == "ocr":
            written = ocr.run_batch(
                args.images, lang, out_dir=args.out_dir,
                on_result=lambda src, dst: print(f"{src} -> {dst}", file=sys.stderr),
            )
            print(f"Wrote {len(written)} text file(s)", file=sys.stderr)
        elif args.command == "tts":
            if args.interactive:
                from languify import interactive  # see module docstring: heavier, optional deps

                interactive.main(args.texts, lang)
            else:
                out_path = tts.run_combine(args.texts, lang, out_path=args.out)
                print(out_path)
    except KeyboardInterrupt:
        sys.exit(130)
    except (FileNotFoundError, KeyError, ValueError, ImportError, RuntimeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
