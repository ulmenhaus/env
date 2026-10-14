"""Non-interactive entry point for `languify accent`: scores one
pre-recorded wav against a combined text, same text-combining convention
as `languify tts`'s default (non `-i`) mode. The curses recording UI
lives in interactive.py; this is for scripting/testing against a file you
already have, without needing a live microphone.
"""

import argparse
import sys
from typing import List

import soundfile as sf

from languify.accent.diff import Op
from languify.accent.pipeline import score_sentence
from languify.tts import read_text_files


def _print_report(reports) -> None:
    for wr in reports:
        print(
            f"=== {wr.word}  [{wr.start:.2f}s-{wr.end:.2f}s]  "
            f"align_confidence={wr.align_confidence:.2f}  "
            f"phone_error_rate={wr.phone_error_rate:.2f} ==="
        )
        for p in wr.phones:
            bits = [f"{p.op.value:11s} target={p.target!r:6} recognized={p.recognized!r:6}"]
            if p.duration_ms is not None:
                bits.append(f"dur={p.duration_ms:.0f}ms")
            if p.formants is not None:
                bits.append(f"F1={p.formants.f1_hz} F2={p.formants.f2_hz}")
            if p.vot_ms is not None:
                bits.append(f"VOT={p.vot_ms:.0f}ms")
            print("  " + "  ".join(bits))
        print()

    total_target_phones = sum(
        1 for wr in reports for p in wr.phones if p.op != Op.INSERT
    )
    total_errors = sum(
        1 for wr in reports for p in wr.phones if p.op in (Op.SUBSTITUTE, Op.DELETE)
    )
    if total_target_phones:
        print(
            f"Overall phone error rate: {total_errors}/{total_target_phones} "
            f"= {total_errors / total_target_phones:.2f}"
        )


def add_arguments(parser: argparse.ArgumentParser) -> None:
    """Shared between this module's standalone parser and languify's
    top-level `accent` subcommand (see __main__.py)."""
    parser.add_argument("-l", "--language", required=True, help="farsi or hebrew")
    parser.add_argument(
        "texts", nargs="+",
        help="text files to read ('-' pastes from the clipboard via txtpaste)",
    )
    parser.add_argument("--input-wav", required=True, help="recording to score")
    parser.add_argument(
        "--nakdimon-path", default=None,
        help="path to the Nakdimon niqqud model (required for Hebrew; see "
             "tts.nakdimon_path in ~/.languify.json)",
    )


def run(language: str, texts: List[str], input_wav: str, nakdimon_path=None) -> None:
    words = "\n\n".join(read_text_files(texts)).split()
    audio, sample_rate = sf.read(input_wav, dtype="float32")
    try:
        reports = score_sentence(
            audio, sample_rate, words, language, nakdimon_path=nakdimon_path
        )
    except (ValueError, RuntimeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        sys.exit(1)
    _print_report(reports)


def main(argv=None) -> None:
    parser = argparse.ArgumentParser(
        prog="languify accent",
        description="Score a recording of a sentence against its target "
                     "pronunciation, word by word and phone by phone.",
    )
    add_arguments(parser)
    args = parser.parse_args(argv)
    run(args.language, args.texts, args.input_wav, args.nakdimon_path)


if __name__ == "__main__":
    main()
