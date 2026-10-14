"""Microphone recording for the accent tool.

Backed by `sounddevice` (PortAudio bindings). NOTE: this module has not
been runtime-tested -- it was written and wired up in a sandboxed dev
environment with no audio input device and no system `libportaudio2`
available to even import the library, let alone open a microphone. The
logic below is straightforward and should work, but verify it against a
real mic before trusting it: `python3 -m sounddevice` lists input
devices, and `apt install libportaudio2` (or your distro's equivalent) is
the usual fix if `import sounddevice` itself fails.
"""

from dataclasses import dataclass

import numpy as np
import sounddevice as sd

SAMPLE_RATE = 16000  # matches recognize.py's model; align.py resamples regardless


@dataclass
class Recording:
    audio: np.ndarray  # 1-D float32, mono
    sample_rate: int


class Recorder:
    """Start/stop recording driven by two separate calls rather than a
    blocking wait -- unlike a plain script, a curses event loop owns the
    keyboard itself (via getch()), so recording has to start on one
    keypress and stop on a later one, with the UI staying responsive
    (and drawable) in between."""

    def __init__(self, sample_rate: int = SAMPLE_RATE):
        self.sample_rate = sample_rate
        self._stream = None
        self._chunks = []

    @property
    def active(self) -> bool:
        return self._stream is not None

    def start(self) -> None:
        if self.active:
            raise RuntimeError("Recorder.start() called while already recording")
        self._chunks = []
        self._stream = sd.InputStream(
            samplerate=self.sample_rate,
            channels=1,
            dtype="float32",
            callback=self._on_audio,
        )
        self._stream.start()

    def _on_audio(self, indata, frames, time_info, status) -> None:
        self._chunks.append(indata.copy())

    def stop(self) -> Recording:
        if not self.active:
            raise RuntimeError("Recorder.stop() called while not recording")
        self._stream.stop()
        self._stream.close()
        self._stream = None
        audio = (
            np.concatenate(self._chunks).flatten()
            if self._chunks
            else np.zeros(0, dtype="float32")
        )
        return Recording(audio=audio, sample_rate=self.sample_rate)


def list_input_devices():
    return [d for d in sd.query_devices() if d["max_input_channels"] > 0]
