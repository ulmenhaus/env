"""Background playback for a generated exercise.

Prefers a real MIDI-capable synth on PATH (fluidsynth or timidity) playing
the actual .mid file. If neither is available -- and neither is installed
in some minimal environments -- falls back to synthesizing a plain sine-tone
.wav directly from the note data with numpy, played through whatever
generic audio player is on PATH. Either way playback runs as a detached
background process so the curses UI stays responsive.
"""

import glob
import os
import shutil
import subprocess
import wave

import numpy as np

from sightread import midi, theory

SOUNDFONT_CANDIDATES = [
    "/usr/share/sounds/sf2/FluidR3_GM.sf2",
    "/usr/share/sounds/sf2/default.sf2",
    "/usr/share/soundfonts/FluidR3_GM.sf2",
    "/usr/share/soundfonts/default.sf2",
]


def _find_soundfont():
    env = os.environ.get("SIGHTREAD_SOUNDFONT")
    if env and os.path.exists(env):
        return env
    for path in SOUNDFONT_CANDIDATES:
        if os.path.exists(path):
            return path
    for pattern in ("/usr/share/sounds/sf2/*.sf2", "/usr/share/soundfonts/*.sf2"):
        matches = glob.glob(pattern)
        if matches:
            return matches[0]
    return None


def _midi_player_command(mid_path):
    if shutil.which("fluidsynth"):
        sf = _find_soundfont()
        if sf:
            return ["fluidsynth", "-i", "-q", "-g", "1.0", sf, mid_path]
    if shutil.which("timidity"):
        return ["timidity", "-q", mid_path]
    if shutil.which("wildmidi"):
        return ["wildmidi", mid_path]
    return None


def _wav_player_command(wav_path):
    for name, args in [
        ("paplay", []),
        ("aplay", ["-q"]),
        ("afplay", []),
        ("play", ["-q"]),
        ("ffplay", ["-nodisp", "-autoexit", "-loglevel", "quiet"]),
    ]:
        if shutil.which(name):
            return [name, *args, wav_path]
    return None


def synthesize_wav(systems, out_path, bpm=76, sample_rate=22050):
    """Render the exercise directly to a WAV with simple plucked-tone sine
    synthesis -- used only when no MIDI-capable player is installed."""
    seconds_per_beat = 60.0 / bpm
    total_beats = 0.0
    note_events = []
    for start_beat, beats, notes in midi.timed_events(systems):
        total_beats = max(total_beats, start_beat + beats)
        if notes:
            note_events.append((start_beat, beats, notes))

    total_samples = int((total_beats * seconds_per_beat + 0.5) * sample_rate) + 1
    buffer = np.zeros(total_samples, dtype=np.float64)

    for start_beat, beats, notes in note_events:
        start_s = start_beat * seconds_per_beat
        dur_s = beats * seconds_per_beat * 0.88
        n = int(dur_s * sample_rate)
        if n <= 0:
            continue
        t = np.arange(n) / sample_rate
        envelope = np.exp(-3.0 * t / max(dur_s, 1e-6))
        start_idx = int(start_s * sample_rate)
        amp = 0.35 / max(1, len(notes))
        for letter, acc, octave in notes:
            pitch = theory.midi_number(letter, acc, octave)
            freq = 440.0 * 2 ** ((pitch - 69) / 12.0)
            tone = np.sin(2 * np.pi * freq * t) * envelope * amp
            end_idx = start_idx + n
            buffer[start_idx:end_idx] += tone[: max(0, total_samples - start_idx)]

    peak = np.max(np.abs(buffer)) or 1.0
    scaled = np.clip(buffer / peak * 0.9, -1.0, 1.0)
    pcm = (scaled * 32767).astype(np.int16)

    with wave.open(out_path, "wb") as wav_file:
        wav_file.setnchannels(1)
        wav_file.setsampwidth(2)
        wav_file.setframerate(sample_rate)
        wav_file.writeframes(pcm.tobytes())

    return out_path


class Player:
    """Plays one exercise at a time in the background; a new play() call
    cuts off whatever was previously sounding."""

    def __init__(self):
        self.proc = None
        self.backend_description = self._describe_backend()

    def _describe_backend(self):
        if shutil.which("fluidsynth") and _find_soundfont():
            return "fluidsynth"
        if shutil.which("timidity"):
            return "timidity"
        if shutil.which("wildmidi"):
            return "wildmidi"
        wav_cmd = _wav_player_command("dummy.wav")
        if wav_cmd:
            return f"synth+{wav_cmd[0]}"
        return None

    def play(self, mid_path, systems, wav_scratch_path):
        self.stop()
        cmd = _midi_player_command(mid_path)
        if cmd is None:
            synthesize_wav(systems, wav_scratch_path)
            cmd = _wav_player_command(wav_scratch_path)
        if cmd is None:
            return False
        self.proc = subprocess.Popen(
            cmd, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        )
        return True

    def stop(self):
        if self.proc is not None and self.proc.poll() is None:
            self.proc.terminate()
        self.proc = None

    def is_playing(self):
        return self.proc is not None and self.proc.poll() is None
