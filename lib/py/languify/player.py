"""Background WAV playback for the interactive reader.

Runs playback as a detached subprocess (same approach as this repo's
sightread.audio.Player) so the curses event loop stays responsive while
audio is sounding -- is_playing() is polled from the main loop to notice
when a sentence has finished and auto-advance to the next one.
"""

import shutil
import subprocess
from typing import Optional

_PLAYER_CANDIDATES = [
    ("paplay", []),
    ("aplay", ["-q"]),
    ("afplay", []),
    ("play", ["-q"]),
    ("ffplay", ["-nodisp", "-autoexit", "-loglevel", "quiet"]),
]


def _player_command(wav_path: str):
    for name, args in _PLAYER_CANDIDATES:
        if shutil.which(name):
            return [name, *args, wav_path]
    return None


class Player:
    def __init__(self):
        self.proc: Optional[subprocess.Popen] = None

    def available(self) -> bool:
        return _player_command("dummy.wav") is not None

    def play(self, wav_path: str) -> bool:
        self.stop()
        cmd = _player_command(wav_path)
        if cmd is None:
            return False
        self.proc = subprocess.Popen(
            cmd, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL
        )
        return True

    def stop(self) -> None:
        if self.proc is not None and self.proc.poll() is None:
            self.proc.terminate()
        self.proc = None

    def is_playing(self) -> bool:
        return self.proc is not None and self.proc.poll() is None
