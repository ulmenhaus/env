"""Curses reader: plays each sentence of the given text file(s) aloud
while bolding it on screen, one page at a time, RTL-aware.

Controls:
  space       play/pause -- playback always (re)starts the current
              sentence from its beginning; there is no mid-sentence
              resume, per design.
  r           restart -- back to the first sentence of the current page
  left/right  previous/next sentence
  h/l         previous/next page
  +/-         speed up/slow down playback, in 0.25x steps (default 1x)
  enter       re-paste any clipboard ("-") source(s) and jump to the
              start of the freshly-pasted text (no-op without one)
  q           quit

Every navigation key (r, arrows, h/l, +/-, enter) pauses first: playback
only ever starts because you pressed space, never as a side effect of
moving around, changing speed, or refreshing the clipboard.

A text argument of "-" pastes from the system clipboard (via this
repo's `txtpaste`) instead of reading a file; enter re-runs it.

Each sentence's audio is synthesized on first visit and the raw samples
are cached in memory for the session, so revisiting a sentence (e.g.
stepping back with the left arrow) does not re-run the model. A speed
other than 1x is applied by pitch-preserving time-stretching (see
timestretch.py) of those cached samples, which is itself cached per
(sentence, speed) pair so replaying a sentence at an unchanged speed
doesn't redo that work either.
"""

import curses
import locale
import os
import shutil
import tempfile
import textwrap
from typing import List

from languify.bidi_render import to_visual
from languify.config import LanguageConfig
from languify.engines import build_engine
from languify.audio_io import write_wav
from languify.player import Player
from languify.textutil import split_sentences
from languify.timestretch import change_speed
from languify.tts import CLIPBOARD_MARKER, read_text

MARGIN = 2
HEADER_ROWS = 2
FOOTER_ROWS = 2
POLL_MS = 100

DEFAULT_SPEED = 1.0
SPEED_STEP = 0.25
MIN_SPEED = 0.25
MAX_SPEED = 3.0


class Reader:
    def __init__(self, stdscr, text_paths: List[str], lang: LanguageConfig):
        self.stdscr = stdscr
        self.text_paths = text_paths
        self.lang = lang
        self.engine = build_engine(lang)
        self.player = Player()
        self.cache_dir = tempfile.mkdtemp(prefix="languify-")
        self.pcm_cache = {}
        self.stretched_cache = {}

        self.current = 0
        self.playing = False
        self.speed = DEFAULT_SPEED
        self.message = ""
        self.pages: List[List[int]] = []
        self.wrapped: List[List[str]] = []

        # Sentences per source (parallel to text_paths), so a clipboard
        # ("-") source can be re-fetched and re-split on its own without
        # disturbing sentences that came from other sources.
        self.source_sentences: List[List[str]] = [
            split_sentences(read_text(path)) for path in text_paths
        ]
        self.sentences: List[str] = []
        self._rebuild_sentences()
        self.layout()

    def _rebuild_sentences(self) -> None:
        self.sentences = [s for group in self.source_sentences for s in group]

    def refresh_clipboard(self) -> None:
        """Re-run txtpaste for every clipboard ("-") source and jump to
        the start of the first one's freshly-pasted text. A no-op if
        none of the sources are "-"."""
        indices = [i for i, p in enumerate(self.text_paths) if p == CLIPBOARD_MARKER]
        if not indices:
            return

        self.pause()
        try:
            for i in indices:
                self.source_sentences[i] = split_sentences(read_text(CLIPBOARD_MARKER))
        except RuntimeError as exc:
            self.message = str(exc)
            return

        jump_to = sum(len(group) for group in self.source_sentences[: indices[0]])
        self._rebuild_sentences()
        self.pcm_cache.clear()
        self.stretched_cache.clear()
        self.current = min(jump_to, max(len(self.sentences) - 1, 0))
        self.message = ""
        self.layout()

    # -- layout -----------------------------------------------------------

    def layout(self) -> None:
        """Wrap every sentence to the current terminal width and pack them
        into pages that fit the current terminal height. Must run again on
        KEY_RESIZE. Wrapping happens on the logical string; bidi reordering
        is applied per-line only at draw time (see draw())."""
        rows, cols = self.stdscr.getmaxyx()
        avail_rows = max(rows - HEADER_ROWS - FOOTER_ROWS, 3)
        avail_cols = max(cols - 2 * MARGIN, 10)

        self.wrapped = [
            textwrap.wrap(sentence, width=avail_cols) or [""]
            for sentence in self.sentences
        ]

        pages: List[List[int]] = []
        page: List[int] = []
        used = 0
        for idx, lines in enumerate(self.wrapped):
            needed = len(lines) + (1 if page else 0)  # +1 for separator row
            if page and used + needed > avail_rows:
                pages.append(page)
                page, used, needed = [], 0, len(lines)
            page.append(idx)
            used += needed
        if page:
            pages.append(page)
        self.pages = pages or [[]]

        if self.sentences:
            self.current = min(self.current, len(self.sentences) - 1)

    def page_of(self, sentence_idx: int) -> int:
        for page_idx, sentence_idxs in enumerate(self.pages):
            if sentence_idx in sentence_idxs:
                return page_idx
        return 0

    # -- drawing ------------------------------------------------------------

    def draw(self) -> None:
        self.stdscr.erase()
        rows, cols = self.stdscr.getmaxyx()
        page_idx = self.page_of(self.current)

        header = f"languify -- {self.lang.name} reader    Page {page_idx + 1}/{len(self.pages)}"
        self._addstr(0, 0, header, cols, curses.A_BOLD)
        state = "playing" if self.playing else "paused"
        backend = "no audio player found on PATH" if not self.player.available() else state
        info = f"[{backend}]  Speed: {self.speed:.2f}x"
        if self.message:
            info += f"  -- {self.message}"
        self._addstr(1, 0, info, cols)

        row = HEADER_ROWS
        if not self.sentences:
            self._addstr(row, MARGIN, "(no text loaded)", cols, curses.A_DIM)
        for sentence_idx in self.pages[page_idx]:
            attr = curses.A_BOLD if sentence_idx == self.current else curses.A_NORMAL
            for line in self.wrapped[sentence_idx]:
                if row >= rows - FOOTER_ROWS:
                    break
                visual = to_visual(line, self.lang.needs_reshaping)
                text = visual[: cols - 2 * MARGIN]
                x = MARGIN
                if self.lang.rtl:
                    x = max(MARGIN, cols - MARGIN - len(text))
                self._addstr(row, x, text, cols, attr)
                row += 1
            row += 1  # blank separator between sentences

        footer = "[space] play/pause  [r] restart page  [<-/->] sentence  [h/l] page  [+/-] speed"
        if CLIPBOARD_MARKER in self.text_paths:
            footer += "  [enter] re-paste"
        footer += "  [q] quit"
        self._addstr(rows - 1, 0, footer, cols, curses.A_DIM)
        self.stdscr.refresh()

    def _addstr(self, row, col, text, cols, attr=curses.A_NORMAL) -> None:
        try:
            self.stdscr.addstr(row, col, text[: max(cols - col, 0)], attr)
        except curses.error:
            pass  # bottom-right cell write; ncurses quirk, harmless to skip

    # -- playback -----------------------------------------------------------

    def _wav_for(self, sentence_idx: int) -> str:
        """Synthesize (or reuse the cached samples for) a sentence, apply
        the current speed as a pitch-preserving time-stretch (or reuse
        that too, if already done at this exact speed), and write out
        its WAV file at the engine's original, unchanged sample rate."""
        pcm = self.pcm_cache.get(sentence_idx)
        if pcm is None:
            pcm = self.engine.synthesize(self.sentences[sentence_idx])
            self.pcm_cache[sentence_idx] = pcm

        cache_key = (sentence_idx, self.speed)
        stretched = self.stretched_cache.get(cache_key)
        if stretched is None:
            stretched = change_speed(pcm, self.speed)
            self.stretched_cache[cache_key] = stretched

        path = os.path.join(self.cache_dir, f"{sentence_idx}.wav")
        write_wav(path, stretched, self.engine.sample_rate)
        return path

    def _start_current(self) -> None:
        self.playing = self.player.play(self._wav_for(self.current))

    def toggle_play(self) -> None:
        if self.playing:
            self.pause()
        elif self.sentences:
            self._start_current()

    def pause(self) -> None:
        self.player.stop()
        self.playing = False

    def restart_page(self) -> None:
        self.pause()
        page = self.pages[self.page_of(self.current)]
        if page:
            self.current = page[0]

    def move(self, delta: int) -> None:
        self.pause()
        new_idx = self.current + delta
        if 0 <= new_idx < len(self.sentences):
            self.current = new_idx

    def change_page(self, delta: int) -> None:
        self.pause()
        new_page = self.page_of(self.current) + delta
        if 0 <= new_page < len(self.pages) and self.pages[new_page]:
            self.current = self.pages[new_page][0]

    def change_speed(self, delta: float) -> None:
        self.pause()
        self.speed = round(min(MAX_SPEED, max(MIN_SPEED, self.speed + delta)), 2)

    def tick(self) -> None:
        """Poll playback; once the current sentence's audio finishes while
        still in the 'playing' state, move on to the next sentence (this is
        what makes the reader read continuously instead of one sentence per
        space-press)."""
        if self.playing and not self.player.is_playing():
            if self.current + 1 < len(self.sentences):
                self.current += 1
                self._start_current()
            else:
                self.playing = False

    def close(self) -> None:
        self.player.stop()
        shutil.rmtree(self.cache_dir, ignore_errors=True)


def _run(stdscr, text_paths: List[str], lang: LanguageConfig) -> None:
    curses.curs_set(0)
    stdscr.timeout(POLL_MS)

    reader = Reader(stdscr, text_paths, lang)
    try:
        reader.draw()
        while True:
            reader.tick()
            ch = stdscr.getch()
            if ch == -1:
                reader.draw()
                continue
            elif ch == curses.KEY_RESIZE:
                reader.layout()
            elif ch in (ord("q"), ord("Q")):
                break
            elif ch == ord(" "):
                reader.toggle_play()
            elif ch in (ord("r"), ord("R")):
                reader.restart_page()
            elif ch == curses.KEY_LEFT:
                reader.move(-1)
            elif ch == curses.KEY_RIGHT:
                reader.move(1)
            elif ch == ord("h"):
                reader.change_page(-1)
            elif ch == ord("l"):
                reader.change_page(1)
            elif ch in (ord("+"), ord("=")):
                reader.change_speed(SPEED_STEP)
            elif ch in (ord("-"), ord("_")):
                reader.change_speed(-SPEED_STEP)
            elif ch in (curses.KEY_ENTER, 10, 13):
                reader.refresh_clipboard()
            reader.draw()
    finally:
        reader.close()


def main(text_paths: List[str], lang: LanguageConfig) -> None:
    locale.setlocale(locale.LC_ALL, "")  # must precede initscr for unicode addstr
    curses.wrapper(_run, text_paths, lang)
