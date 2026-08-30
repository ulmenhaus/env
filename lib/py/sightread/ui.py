"""Curses front-end: shows a status bar in the top few terminal rows and
renders the staff image below it with `kitty +kitten icat`. Curses never
draws into the image's rows, so redrawing the status bar does not clobber
the picture kitty placed there (the same split used by bin/term-doc-viewer).
"""

import curses
import json
import os
import random
import shutil
import subprocess
import sys

from sightread import audio, generator, midi, render

STATE_DIR = os.path.expanduser("~/.cache/sightread")
CONFIG_PATH = os.path.expanduser("~/.config/sightread/state.json")
IMAGE_ROW = 8


def _load_level():
    try:
        with open(CONFIG_PATH) as f:
            return max(1, min(generator.NUM_LEVELS, int(json.load(f).get("level", 1))))
    except (OSError, ValueError, TypeError, json.JSONDecodeError):
        return 1


def _save_level(level):
    os.makedirs(os.path.dirname(CONFIG_PATH), exist_ok=True)
    with open(CONFIG_PATH, "w") as f:
        json.dump({"level": level}, f)


class ImageDisplay:
    def __init__(self):
        self.available = shutil.which("kitty") is not None

    def clear(self):
        if not self.available:
            return
        # icat's output *is* the terminal escape-code protocol that makes the
        # image appear -- it must reach the real stdout, not be discarded.
        subprocess.run(["kitty", "+kitten", "icat", "--clear"])

    def show(self, path):
        if not self.available:
            return
        self.clear()
        sys.stdout.write(f"\x1b[{IMAGE_ROW};1H")
        sys.stdout.flush()
        subprocess.run(["kitty", "+kitten", "icat", "--transfer-mode=file", path])


def run(stdscr, initial_level=None):
    curses.curs_set(0)
    os.makedirs(STATE_DIR, exist_ok=True)

    # ncurses has no record of the terminal's real contents until its first
    # refresh, so that first refresh always does a full physical clear
    # (rather than a diff) to establish a baseline. Do it now, on an empty
    # screen, so it can never land after an image is placed and erase it.
    stdscr.erase()
    stdscr.refresh()

    level = initial_level or _load_level()
    rng = random.Random()
    player = audio.Player()
    display = ImageDisplay()
    sample = {}

    def new_sample():
        key, ts, systems, etype = generator.generate_sample(level, rng)
        img_path = os.path.join(STATE_DIR, "current.png")
        mid_path = os.path.join(STATE_DIR, "current.mid")
        wav_path = os.path.join(STATE_DIR, "current.wav")
        title = f"Level {level}   {key.name}   {etype}   {ts[0]}/{ts[1]}"
        render.render_exercise(systems, key, ts[0], ts[1], img_path, title=title)
        midi.write_midi(systems, mid_path)
        sample.update(key=key, ts=ts, systems=systems, etype=etype,
                       img=img_path, mid=mid_path, wav=wav_path)

    def advance(message=""):
        player.stop()
        new_sample()
        display.show(sample["img"])
        draw_status(message)

    def draw_status(message=""):
        stdscr.erase()
        stdscr.addstr(0, 0, "sightread -- guitar sight-reading trainer", curses.A_BOLD)
        stdscr.addstr(1, 0, f"Level {level}/{generator.NUM_LEVELS}   {sample['key'].name}   "
                             f"{sample['etype']}   {sample['ts'][0]}/{sample['ts'][1]}")
        backend = player.backend_description or "none found -- notation only, no audio"
        stdscr.addstr(2, 0, f"Audio backend: {backend}")
        if not display.available:
            stdscr.addstr(3, 0, f"kitty not found -- staff written to {sample['img']}")
        stdscr.addstr(5, 0, "[space] next exercise   [p] play   [s] stop   "
                             "[+/-] level   [q] quit")
        if message:
            stdscr.addstr(6, 0, message)
        stdscr.refresh()

    advance()

    try:
        while True:
            code = stdscr.getch()
            ch = chr(code) if 0 <= code < 256 else ""

            if ch == "q":
                break
            elif ch in (" ", "n"):
                advance()
            elif ch == "p":
                ok = player.play(sample["mid"], sample["systems"], sample["wav"])
                draw_status("" if ok else "no audio backend available")
            elif ch == "s":
                player.stop()
                draw_status()
            elif ch in ("+", "="):
                if level < generator.NUM_LEVELS:
                    level += 1
                    _save_level(level)
                    advance()
                else:
                    draw_status()
            elif ch in ("-", "_"):
                if level > 1:
                    level -= 1
                    _save_level(level)
                    advance()
                else:
                    draw_status()
    finally:
        player.stop()
        display.clear()


def main(initial_level=None):
    curses.wrapper(run, initial_level)
