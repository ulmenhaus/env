"""Default (non-interactive) text-to-speech: combine a batch of text
files and synthesize them to one audio file.
"""

import os
import subprocess
from typing import List, Optional

from languify.audio_io import write_wav
from languify.config import LanguageConfig
from languify.engines import build_engine

CLIPBOARD_MARKER = "-"
CLIPBOARD_COMMAND = ["txtpaste"]


def read_clipboard() -> str:
    """Run this repo's `txtpaste` (bin/txtpaste -- an OS-agnostic clipboard
    reader) and return what's on the clipboard. Requires bin/ on PATH."""
    try:
        result = subprocess.run(
            CLIPBOARD_COMMAND, capture_output=True, text=True, check=True,
        )
    except FileNotFoundError as exc:
        raise RuntimeError(
            "'txtpaste' not found on PATH -- add this repo's bin/ to PATH "
            "(see README.md)"
        ) from exc
    except subprocess.CalledProcessError as exc:
        raise RuntimeError(
            f"txtpaste failed: {exc.stderr.strip() or exc}"
        ) from exc
    return result.stdout


def read_text(path: str) -> str:
    """Read one text argument -- CLIPBOARD_MARKER ("-") pastes from the
    system clipboard via txtpaste, anything else is a file path."""
    if path == CLIPBOARD_MARKER:
        return read_clipboard()
    with open(path, encoding="utf-8") as f:
        return f.read()


def read_text_files(paths: List[str]) -> List[str]:
    return [read_text(path) for path in paths]


def default_output_path(paths: List[str]) -> str:
    base = os.path.splitext(os.path.basename(paths[0]))[0]
    return base + ".wav"


def run_combine(
    paths: List[str], lang: LanguageConfig, out_path: Optional[str] = None
) -> str:
    if out_path is None:
        if len(paths) != 1 or paths[0] == CLIPBOARD_MARKER:
            raise ValueError(
                "--out is required when combining more than one text file, "
                "or when pasting from the clipboard (-)"
            )
        out_path = default_output_path(paths)

    engine = build_engine(lang)
    combined = "\n\n".join(read_text_files(paths))
    pcm = engine.synthesize(combined)
    write_wav(out_path, pcm, engine.sample_rate)
    return out_path
